// Copyright (C) 2021 ScyllaDB

package localdriver

import (
	"context"
	"fmt"
	"path/filepath"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	kubeframework "k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2evolume "k8s.io/kubernetes/test/e2e/framework/volume"
	storageframework "k8s.io/kubernetes/test/e2e/storage/framework"
	storageutils "k8s.io/kubernetes/test/e2e/storage/utils"
	admissionapi "k8s.io/pod-security-admission/api"
	"k8s.io/utils/pointer"
)

const (
	mountPath = "/mnt"
	quota     = storageframework.MinFileSize
)

var _ = g.Describe("XFS Quotas", func() {
	defer g.GinkgoRecover()

	driver := &localCsiDriver{}

	var (
		ctx       context.Context
		ctxCancel context.CancelFunc

		config        *storageframework.PerTestConfig
		driverCleanup func()

		resource *storageframework.VolumeResource

		testPattern = storageframework.TestPattern{
			Name:    "quotas",
			VolType: storageframework.DynamicPV,
			FsType:  "xfs",
		}
	)

	f := kubeframework.NewFrameworkWithCustomTimeouts("quotas", storageframework.GetDriverTimeouts(driver))
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	init := func() {
		ctx, ctxCancel = context.WithCancel(context.Background())
		config, driverCleanup = driver.PrepareTest(f)

		testVolumeSizeRange := e2evolume.SizeRange{Min: fmt.Sprintf("%d", quota)}
		resource = storageframework.CreateVolumeResource(driver, config, testPattern, testVolumeSizeRange)
		if resource.VolSource == nil {
			kubeframework.Fail("Expected driver to define volumeSource")
		}
	}

	cleanup := func() {
		ctxCancel()

		var errs []error
		if resource != nil {
			errs = append(errs, resource.CleanupResource())
			resource = nil
		}

		if driverCleanup != nil {
			errs = append(errs, storageutils.TryFunc(driverCleanup))
			driverCleanup = nil
		}
		o.Expect(errors.NewAggregate(errs)).NotTo(o.HaveOccurred())
	}

	g.It("should fail to write data when volume quota is already reached", func() {
		init()
		defer cleanup()

		config := storageframework.ConvertTestConfig(config)

		initFile := filepath.Join(mountPath, fmt.Sprintf("%s-%s-dd_if", config.Prefix, config.Namespace))
		initCmd := fmt.Sprintf("dd if=/dev/urandom bs=%d count=%d iflag=fullblock of=%s", storageframework.MinFileSize, quota/storageframework.MinFileSize, initFile)
		clientPod := makePodSpec(config, initCmd, *resource.VolSource)

		g.By(fmt.Sprintf("starting %s", clientPod.Name))
		clientPod, err := f.ClientSet.CoreV1().Pods(config.Namespace).Create(ctx, clientPod, metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		defer e2epod.DeletePodWithWait(f.ClientSet, clientPod)

		err = e2epod.WaitTimeoutForPodRunningInNamespace(f.ClientSet, clientPod.Name, clientPod.Namespace, f.Timeouts.PodStart)
		o.Expect(err).NotTo(o.HaveOccurred())

		testFile := filepath.Join(mountPath, fmt.Sprintf("io-%d", storageframework.MinFileSize))
		defer func() {
			deleteFile(f, clientPod, testFile)
		}()

		g.By(fmt.Sprintf("writing %d bytes to test file %s", storageframework.MinFileSize, testFile))
		writeCmd := fmt.Sprintf("dd if=%s bs=%d count=1 of=%s", initFile, storageframework.MinFileSize, testFile)
		_, stderr, err := e2evolume.PodExec(f, clientPod, writeCmd)
		o.Expect(err).To(o.HaveOccurred())
		o.Expect(stderr).To(o.Or(o.ContainSubstring("quota exceeded"), o.ContainSubstring("No space left on device")))
	})
})

func makePodSpec(config e2evolume.TestConfig, initCmd string, volSrc corev1.VolumeSource) *corev1.Pod {
	volName := fmt.Sprintf("volume-%s", config.Namespace)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: config.Prefix + "-client",
		},
		Spec: corev1.PodSpec{
			InitContainers: func() []corev1.Container {
				if len(initCmd) == 0 {
					return nil
				}
				return []corev1.Container{
					{
						Name:  config.Prefix + "-init",
						Image: e2epod.GetDefaultTestImage(),
						Command: []string{
							"/bin/sh",
							"-c",
							initCmd,
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      volName,
								MountPath: mountPath,
							},
						},
					},
				}
			}(),
			Containers: []corev1.Container{
				{
					Name:  config.Prefix + "-client",
					Image: e2epod.GetDefaultTestImage(),
					Command: []string{
						"/bin/sh",
						"-c",
						"sleep 3600",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      volName,
							MountPath: mountPath,
						},
					},
				},
			},
			TerminationGracePeriodSeconds: pointer.Int64(1),
			Volumes: []corev1.Volume{
				{
					Name:         volName,
					VolumeSource: volSrc,
				},
			},
			// Fail if init container fails
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	e2epod.SetNodeSelection(&pod.Spec, config.ClientNodeSelection)
	return pod
}

func deleteFile(f *kubeframework.Framework, pod *corev1.Pod, fpath string) {
	stdout, stderr, err := e2evolume.PodExec(f, pod, fmt.Sprintf("rm -f %s", fpath))
	if err != nil {
		kubeframework.Logf("unable to delete test file %s: %v\nerror ignored, continuing test\nstdout: %s\nstderr: %s", fpath, err, stdout, stderr)
	}
}
