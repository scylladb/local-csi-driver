// Copyright (c) 2023 ScyllaDB.

package localdriver

import (
	"context"
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver"
	"github.com/scylladb/k8s-local-volume-provisioner/pkg/driver/volume"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeframework "k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2evolume "k8s.io/kubernetes/test/e2e/framework/volume"
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	admissionapi "k8s.io/pod-security-admission/api"
)

const (
	capacityPollInterval = 30 * time.Second
)

var _ = g.Describe("Node Capacity", func() {
	defer g.GinkgoRecover()

	d := &localCsiDriver{}

	f := kubeframework.NewFrameworkWithCustomTimeouts("capacity", storageframework.GetDriverTimeouts(d))
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	g.It("should calculate node capacity [Serial]", func() {
		ctx, ctxCancel := context.WithCancel(context.Background())
		defer ctxCancel()

		frameworkTestConfig := d.PrepareTest(ctx, f)

		testVolumeSizeRange := e2evolume.SizeRange{Min: fmt.Sprintf("%d", quota)}
		testPattern := storageframework.TestPattern{
			Name:    "capacity",
			VolType: storageframework.DynamicPV,
			FsType:  "",
		}

		volResource := storageframework.CreateVolumeResource(ctx, d, frameworkTestConfig, testPattern, testVolumeSizeRange)
		o.Expect(volResource.VolSource).NotTo(o.BeNil())

		defer func() {
			cleanupCtx, cleanupCtxCancel := context.WithCancel(context.Background())
			defer cleanupCtxCancel()
			err := volResource.CleanupResource(cleanupCtx)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()

		driverNamespace := frameworkTestConfig.DriverNamespace.Name
		testStorageClassName := volResource.Sc.Name
		testConfig := storageframework.ConvertTestConfig(frameworkTestConfig)

		nodesInCluster, err := f.ClientSet.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("Awaiting CSIStorageCapacity to be available for all nodes in the cluster")

		prePodCapacities := make(map[string]*resource.Quantity, len(nodesInCluster.Items))
		o.Eventually(func() []storagev1.CSIStorageCapacity {
			csiStorageCapacities, err := f.ClientSet.StorageV1().CSIStorageCapacities(driverNamespace).List(ctx, metav1.ListOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())

			var ourCsc []storagev1.CSIStorageCapacity
			for _, csc := range csiStorageCapacities.Items {
				if csc.StorageClassName != testStorageClassName {
					continue
				}

				ourCsc = append(ourCsc, csc)
			}

			return ourCsc
		}).WithTimeout(2 * capacityPollInterval).WithPolling(time.Second).Should(o.HaveLen(len(nodesInCluster.Items)))

		csiStorageCapacities, err := f.ClientSet.StorageV1().CSIStorageCapacities(driverNamespace).List(ctx, metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		for _, csc := range csiStorageCapacities.Items {
			if csc.StorageClassName != testStorageClassName {
				continue
			}

			o.Expect(csc.Capacity).ToNot(o.BeNil())
			o.Expect(csc.NodeTopology.MatchLabels).To(o.HaveKey(driver.NodeNameTopologyKey))

			nodeName := csc.NodeTopology.MatchLabels[driver.NodeNameTopologyKey]
			prePodCapacities[nodeName] = csc.Capacity
		}

		testPod := makePodSpec(testConfig, "", *volResource.VolSource)

		g.By(fmt.Sprintf("Creating test pod %s with volume", testPod.Name))
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
		o.Expect(nodeNameWhereTestPodLanded).ToNot(o.BeEmpty())
		o.Expect(prePodCapacities).To(o.HaveKey(nodeNameWhereTestPodLanded))

		var postPodCapacity *resource.Quantity
		g.By(fmt.Sprintf("awaiting CSIStorageCapacity to be updated from %s", prePodCapacities[nodeNameWhereTestPodLanded]))

		o.Eventually(func() bool {
			csiStorageCapacities, err = f.ClientSet.StorageV1().CSIStorageCapacities(driverNamespace).List(ctx, metav1.ListOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())

			for _, csc := range csiStorageCapacities.Items {
				if csc.StorageClassName != testStorageClassName {
					continue
				}

				nodeName := csc.NodeTopology.MatchLabels[driver.NodeNameTopologyKey]
				if nodeName != nodeNameWhereTestPodLanded {
					continue
				}

				postPodCapacity = csc.Capacity
				break
			}

			o.Expect(postPodCapacity).ToNot(o.BeNil())
			return postPodCapacity.Cmp(*prePodCapacities[nodeNameWhereTestPodLanded]) == -1
		}).WithTimeout(2 * capacityPollInterval).WithPolling(time.Second).Should(o.BeTrue())

		g.By(fmt.Sprintf("CSIStorageCapacity has been updated to %s", postPodCapacity))

		diff := prePodCapacities[nodeNameWhereTestPodLanded].DeepCopy()
		diff.Sub(*postPodCapacity)

		// Node capacity should be reduced by requested volume size and space reserved for next volume metadata.
		expectedDiff := volResource.Pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		expectedDiff.Add(*resource.NewQuantity(volume.MetadataFileMaxSize, resource.DecimalSI))
		o.Expect(diff.Equal(expectedDiff)).To(o.BeTrue())

		g.By("Checking if capacity is aligned when volume is removed")

		g.By("deleting test pod")
		err = e2epod.DeletePodWithWait(ctx, f.ClientSet, testPod)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("deleting test pod pvc")
		err = volResource.CleanupResource(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Node storage capacity should go back to initial cap
		o.Eventually(func() bool {
			csiStorageCapacities, err = f.ClientSet.StorageV1().CSIStorageCapacities(driverNamespace).List(ctx, metav1.ListOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			var postPodDeletionCapacity *resource.Quantity

			for _, csc := range csiStorageCapacities.Items {
				if csc.StorageClassName != testStorageClassName {
					continue
				}

				nodeName := csc.NodeTopology.MatchLabels[driver.NodeNameTopologyKey]
				if nodeName != nodeNameWhereTestPodLanded {
					continue
				}

				postPodDeletionCapacity = csc.Capacity
				break
			}

			o.Expect(postPodDeletionCapacity).ToNot(o.BeNil())

			return postPodDeletionCapacity.Equal(*prePodCapacities[nodeNameWhereTestPodLanded])
		}).WithTimeout(2 * capacityPollInterval).WithPolling(time.Second).Should(o.BeTrue())
	})
})
