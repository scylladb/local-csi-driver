// Copyright (c) 2024 ScyllaDB.

package localdriver

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeletmetrics "k8s.io/kubernetes/pkg/kubelet/metrics"
	kubeframework "k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/metrics"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2evolume "k8s.io/kubernetes/test/e2e/framework/volume"
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

var _ = g.Describe("Metrics", func() {
	defer g.GinkgoRecover()

	d := &localCsiDriver{}

	f := kubeframework.NewFrameworkWithCustomTimeouts("metrics", storageframework.GetDriverTimeouts(d))
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	g.It("should populate volume stats as kubelet metrics", func() {
		ctx, ctxCancel := context.WithCancel(context.Background())
		defer ctxCancel()

		frameworkTestConfig := d.PrepareTest(ctx, f)

		quotaBytes := storageframework.FileSizeMedium
		testVolumeSizeRange := e2evolume.SizeRange{Min: fmt.Sprintf("%d", quotaBytes)}
		testPattern := storageframework.TestPattern{
			Name:    "metrics",
			VolType: storageframework.DynamicPV,
			FsType:  "",
		}

		metricsGrabber, err := metrics.NewMetricsGrabber(ctx, f.ClientSet, nil, f.ClientConfig(), true, false, false, false, false, false)
		o.Expect(err).NotTo(o.HaveOccurred())

		volResource := storageframework.CreateVolumeResource(ctx, d, frameworkTestConfig, testPattern, testVolumeSizeRange)
		o.Expect(volResource.VolSource).NotTo(o.BeNil())

		defer func() {
			cleanupCtx, cleanupCtxCancel := context.WithCancel(context.Background())
			defer cleanupCtxCancel()
			err := volResource.CleanupResource(cleanupCtx)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		testConfig := storageframework.ConvertTestConfig(frameworkTestConfig)
		testPod := makePodSpec(testConfig, "", *volResource.VolSource)

		g.By(fmt.Sprintf("Creating test Pod %q with %q PVC as a volume", testPod.Name, volResource.Pvc.Name))
		testPod, err = f.ClientSet.CoreV1().Pods(testConfig.Namespace).Create(ctx, testPod, metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())

		defer func() {
			cleanupCtx, cleanupCtxCancel := context.WithCancel(context.Background())
			defer cleanupCtxCancel()
			g.By("deleting test pod")
			err = e2epod.DeletePodWithWait(cleanupCtx, f.ClientSet, testPod)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		err = e2epod.WaitTimeoutForPodRunningInNamespace(ctx, f.ClientSet, testPod.Name, testPod.Namespace, f.Timeouts.PodStart)
		o.Expect(err).NotTo(o.HaveOccurred())

		testPod, err = f.ClientSet.CoreV1().Pods(testConfig.Namespace).Get(ctx, testPod.Name, metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By(fmt.Sprintf("Pod landed on %q node", testPod.Spec.NodeName))

		nodeNameWhereTestPodLanded := testPod.Spec.NodeName
		o.Expect(nodeNameWhereTestPodLanded).NotTo(o.BeEmpty())

		volumeStats := map[string]float64{
			kubeletmetrics.VolumeStatsUsedBytesKey:      0,
			kubeletmetrics.VolumeStatsCapacityBytesKey:  0,
			kubeletmetrics.VolumeStatsAvailableBytesKey: 0,
			kubeletmetrics.VolumeStatsInodesKey:         0,
			kubeletmetrics.VolumeStatsInodesFreeKey:     0,
			kubeletmetrics.VolumeStatsInodesUsedKey:     0,
		}

		g.By("Awaiting kubelet metrics to be picked up by the volume stats collector")
		var km metrics.KubeletMetrics
		o.Eventually(func(g o.Gomega) {
			km, err = metricsGrabber.GrabFromKubelet(ctx, nodeNameWhereTestPodLanded)
			g.Expect(err).NotTo(o.HaveOccurred())

			for volumeStatKey := range volumeStats {
				kubeletKeyName := fmt.Sprintf("%s_%s", kubeletmetrics.KubeletSubsystem, volumeStatKey)
				g.Expect(km).To(o.HaveKey(kubeletKeyName))

				i := slices.IndexFunc(km[kubeletKeyName], func(sample *model.Sample) bool {
					return string(sample.Metric["namespace"]) == volResource.Pvc.Namespace &&
						string(sample.Metric["persistentvolumeclaim"]) == volResource.Pvc.Name
				})
				g.Expect(i).To(o.BeNumerically(">=", 0))

				volumeStats[volumeStatKey] = float64(km[kubeletKeyName][i].Value)
			}
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(o.Succeed())

		totalBytes := volumeStats[kubeletmetrics.VolumeStatsCapacityBytesKey]
		usedBytes := volumeStats[kubeletmetrics.VolumeStatsUsedBytesKey]
		availableBytes := volumeStats[kubeletmetrics.VolumeStatsAvailableBytesKey]

		o.Expect(usedBytes).To(o.BeNumerically("==", 0))
		o.Expect(totalBytes).To(o.BeNumerically("==", quotaBytes))
		o.Expect(availableBytes).To(o.BeNumerically("==", quotaBytes))

		// Inodes are not limited by the driver, so these are global limits.
		// Only check if they make sense, as we cannot predict how inodes changes on running system,
		// especially in parallel suite this test might be running.
		totalInodes := volumeStats[kubeletmetrics.VolumeStatsInodesKey]
		usedInodes := volumeStats[kubeletmetrics.VolumeStatsInodesUsedKey]
		availableInodes := volumeStats[kubeletmetrics.VolumeStatsInodesFreeKey]

		o.Expect(totalInodes).To(o.BeNumerically(">", 0))
		o.Expect(usedInodes).To(o.BeNumerically(">", 0))
		o.Expect(availableInodes).To(o.BeNumerically(">", 0))
		o.Expect(availableInodes + usedInodes).To(o.BeNumerically("==", totalInodes))

		writtenBytes := storageframework.MinFileSize
		testFile := filepath.Join(mountPath, fmt.Sprintf("io-%d", writtenBytes))
		g.By(fmt.Sprintf("Writing %d bytes to file on test volume", writtenBytes))
		writeCmd := fmt.Sprintf("dd if=/dev/urandom bs=%d count=1 of=%s", writtenBytes, testFile)
		_, _, err = e2epod.ExecShellInPodWithFullOutput(ctx, f, testPod.Name, writeCmd)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Awaiting kubelet metrics to be updated after write")
		o.Eventually(func(g o.Gomega) {
			km, err = metricsGrabber.GrabFromKubelet(ctx, nodeNameWhereTestPodLanded)
			g.Expect(err).NotTo(o.HaveOccurred())

			usedBytesKey := fmt.Sprintf("%s_%s", kubeletmetrics.KubeletSubsystem, kubeletmetrics.VolumeStatsUsedBytesKey)
			capacityBytesKey := fmt.Sprintf("%s_%s", kubeletmetrics.KubeletSubsystem, kubeletmetrics.VolumeStatsCapacityBytesKey)
			availableBytesKey := fmt.Sprintf("%s_%s", kubeletmetrics.KubeletSubsystem, kubeletmetrics.VolumeStatsAvailableBytesKey)

			volumeMetricHasValue := func(value int64) func(sample *model.Sample) bool {
				return func(sample *model.Sample) bool {
					return string(sample.Metric["namespace"]) == volResource.Pvc.Namespace &&
						string(sample.Metric["persistentvolumeclaim"]) == volResource.Pvc.Name &&
						int64(sample.Value) == value
				}
			}

			g.Expect(km[usedBytesKey]).To(o.ContainElement(o.Satisfy(volumeMetricHasValue(writtenBytes))))
			g.Expect(km[capacityBytesKey]).To(o.ContainElement(o.Satisfy(volumeMetricHasValue(quotaBytes))))
			g.Expect(km[availableBytesKey]).To(o.ContainElement(o.Satisfy(volumeMetricHasValue(quotaBytes - writtenBytes))))
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(o.Succeed())
	})
})
