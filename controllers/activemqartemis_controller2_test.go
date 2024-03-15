/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// +kubebuilder:docs-gen:collapse=Apache License

/*
As usual, we start with the necessary imports. We also define some utility variables.
*/
package controllers

import (
	"container/list"
	"fmt"
	"os"
	"strconv"

	"github.com/artemiscloud/activemq-artemis-operator/pkg/resources/volumes"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/common"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/namer"

	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/watch"

	brokerv1beta1 "github.com/artemiscloud/activemq-artemis-operator/api/v1beta1"
)

var _ = Describe("artemis controller 2", func() {

	// see what has changed from the controllers perspective, what we watch
	toWatch := []client.ObjectList{&brokerv1beta1.ActiveMQArtemisList{}, &appsv1.StatefulSetList{}, &corev1.PodList{}}
	wis := list.New()
	BeforeEach(func() {

		BeforeEachSpec()

		if verbose {
			fmt.Println("Time with MicroSeconds: ", time.Now().Format("2006-01-02 15:04:05.000000"), " test:", CurrentSpecReport())
		}

		for _, li := range toWatch {

			wc, err := client.NewWithWatch(testEnv.Config, client.Options{})
			if err != nil {
				fmt.Printf("Err on watch client:  %v\n", err)
				return
			}

			// see what changed
			wi, err := wc.Watch(ctx, li, &client.ListOptions{})
			if err != nil {
				fmt.Printf("Err on watch:  %v\n", err)
			}
			wis.PushBack(wi)

			go func() {
				for event := range wi.ResultChan() {
					if verbose {
						fmt.Printf("%v : Object: %v\n", event.Type, event.Object)
					}
				}
			}()
		}
	})

	AfterEach(func() {
		for e := wis.Front(); e != nil; e = e.Next() {
			e.Value.(watch.Interface).Stop()
		}

		AfterEachSpec()
	})

	Context("persistent volumes tests", Label("controller-2-test"), func() {
		It("external volumes attach", func() {
			if os.Getenv("USE_EXISTING_CLUSTER") == "true" {

				By("create extra volume")
				pvc, createdPvc := DeployCustomPVC("my-pvc", defaultNamespace, func(candidate *corev1.PersistentVolumeClaim) {

					candidate.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					}
					candidate.Spec.Resources = corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Mi"),
						},
					}
				})

				By("use a pod to write some files to the volume")
				brokerCr, createdBrokerCr := DeployCustomBroker(defaultNamespace, func(candidate *brokerv1beta1.ActiveMQArtemis) {
					candidate.Spec.DeploymentPlan.Size = common.Int32ToPtr(1)
					candidate.Spec.DeploymentPlan.ReadinessProbe = &corev1.Probe{
						InitialDelaySeconds: 1,
						PeriodSeconds:       1,
						TimeoutSeconds:      5,
					}
					candidate.Spec.DeploymentPlan.ExtraVolumes = []corev1.Volume{
						{
							Name: "mydata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-pvc",
								},
							},
						},
					}
				})

				By("waiting for pod ready")
				WaitForPod(brokerCr.Name, 0)

				By("writing some files to the volume")
				podWithOrdinal := namer.CrToSS(brokerCr.Name) + "-0"

				newFileCmd := []string{"/bin/sh", "-c", "echo Hello-World > /amq/extra/volumes/mydata/fake.config"}
				content, err := RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", newFileCmd)
				Expect(err).To(BeNil())
				Expect(*content).To(BeEmpty())

				By("check the file exists")
				lsCmd := []string{"ls", "/amq/extra/volumes/mydata"}
				content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", lsCmd)
				Expect(err).To(BeNil())
				Expect(*content).To(ContainSubstring("fake.config"))

				By("shut down the pod")
				CleanResource(createdBrokerCr, createdBrokerCr.Name, defaultNamespace)

				By("deploying 2 brokers to use existing volume")
				brokerCr, createdBrokerCr = DeployCustomBroker(defaultNamespace, func(candidate *brokerv1beta1.ActiveMQArtemis) {
					candidate.Spec.DeploymentPlan.Size = common.Int32ToPtr(2)
					candidate.Spec.DeploymentPlan.ReadinessProbe = &corev1.Probe{
						InitialDelaySeconds: 1,
						PeriodSeconds:       1,
						TimeoutSeconds:      5,
					}
					candidate.Spec.DeploymentPlan.ExtraVolumes = []corev1.Volume{
						{
							Name: "mydata",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "my-pvc",
								},
							},
						},
					}
					candidate.Spec.DeploymentPlan.ExtraVolumeMounts = []corev1.VolumeMount{
						{
							Name:      "mydata",
							MountPath: "/opt/common",
						},
					}
				})

				By("waiting 2 pods ready")
				WaitForPod(brokerCr.Name, 0, 1)

				By("checking the file still exist on volumes")
				lsCmd = []string{"ls", "/opt/common"}
				catCmd := []string{"cat", "/opt/common/fake.config"}
				for podOrdinal := range []int32{0, 1} {
					podWithOrdinal = namer.CrToSS(brokerCr.Name) + "-" + strconv.Itoa(podOrdinal)
					content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", lsCmd)
					Expect(err).To(BeNil())
					Expect(*content).To(ContainSubstring("fake.config"), *content)

					content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", catCmd)
					Expect(err).To(BeNil())
					Expect(*content).To(ContainSubstring("Hello-World"))
				}

				CleanResource(createdBrokerCr, createdBrokerCr.Name, defaultNamespace)
				CleanResource(createdPvc, pvc.Name, pvc.Namespace)
			}
		})

		It("external pvc with pvc templates", func() {
			if os.Getenv("USE_EXISTING_CLUSTER") == "true" {

				brokerCrName := NextSpecResourceName()

				pvcName := "mydata-" + namer.CrToSS(brokerCrName) + "-0"
				By("create extra pvc: " + pvcName)
				pvc, createdPvc := DeployCustomPVC(pvcName, defaultNamespace, func(candidate *corev1.PersistentVolumeClaim) {

					candidate.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					}
					candidate.Spec.Resources = corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("2Mi"),
						},
					}
				})

				By("use a pod to write some files to the volume")
				brokerCr, createdBrokerCr := DeployCustomBroker(defaultNamespace, func(candidate *brokerv1beta1.ActiveMQArtemis) {
					candidate.Name = brokerCrName
					candidate.Spec.DeploymentPlan.Size = common.Int32ToPtr(1)
					candidate.Spec.DeploymentPlan.ReadinessProbe = &corev1.Probe{
						InitialDelaySeconds: 1,
						PeriodSeconds:       1,
						TimeoutSeconds:      5,
					}
					candidate.Spec.DeploymentPlan.ExtraVolumeClaimTemplates = []brokerv1beta1.VolumeClaimTemplate{
						{
							ObjectMeta: brokerv1beta1.ObjectMeta{
								Name: "mydata",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("2Mi"),
									},
								},
							},
						},
					}
				})

				defaultMoutPath := volumes.GetDefaultExternalPVCMountPath("mydata")

				By("waiting for pod ready")
				WaitForPod(brokerCr.Name, 0)

				By("writing some files to the volume")
				podWithOrdinal := namer.CrToSS(brokerCr.Name) + "-0"

				newFileCmd := []string{"/bin/sh", "-c", "echo my-data-pod-0 > " + *defaultMoutPath + "/fake.data"}
				content, err := RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", newFileCmd)
				Expect(err).To(BeNil())
				Expect(*content).To(BeEmpty())

				By("check the file exists")
				lsCmd := []string{"ls", *defaultMoutPath}
				content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", lsCmd)
				Expect(err).To(BeNil())
				Expect(*content).To(ContainSubstring("fake.data"))

				By("shut down the pod")
				CleanResource(createdBrokerCr, createdBrokerCr.Name, defaultNamespace)

				By("deploying 2 brokers")
				mountPath := "/opt/mydata"
				brokerCr, createdBrokerCr = DeployCustomBroker(defaultNamespace, func(candidate *brokerv1beta1.ActiveMQArtemis) {
					candidate.Name = brokerCrName
					candidate.Spec.DeploymentPlan.Size = common.Int32ToPtr(2)
					candidate.Spec.DeploymentPlan.ReadinessProbe = &corev1.Probe{
						InitialDelaySeconds: 1,
						PeriodSeconds:       1,
						TimeoutSeconds:      5,
					}
					candidate.Spec.DeploymentPlan.ExtraVolumeClaimTemplates = []brokerv1beta1.VolumeClaimTemplate{
						{
							ObjectMeta: brokerv1beta1.ObjectMeta{
								Name: "mydata",
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceName(corev1.ResourceStorage): resource.MustParse("2Mi"),
									},
								},
							},
						},
					}
					candidate.Spec.DeploymentPlan.ExtraVolumeMounts = []corev1.VolumeMount{
						{
							Name:      "mydata",
							MountPath: mountPath,
						},
					}
				})

				By("waiting 2 pods ready")
				WaitForPod(brokerCr.Name, 0, 1)

				By("checking the file exists on pod 0")
				lsCmd = []string{"ls", mountPath}
				catCmd := []string{"cat", mountPath + "/fake.data"}

				podWithOrdinal = namer.CrToSS(brokerCr.Name) + "-0"
				content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", lsCmd)
				Expect(err).To(BeNil())
				Expect(*content).To(ContainSubstring("fake.data"))

				content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", catCmd)
				Expect(err).To(BeNil())
				Expect(*content).To(ContainSubstring("my-data-pod-0"))

				By("checking the file not exist on pod 1")
				lsCmd = []string{"ls", mountPath}

				podWithOrdinal = namer.CrToSS(brokerCr.Name) + "-1"
				content, err = RunCommandInPod(podWithOrdinal, brokerCr.Name+"-container", lsCmd)
				Expect(err).To(BeNil())
				Expect(*content).NotTo(ContainSubstring("fake.data"))

				CleanResource(createdBrokerCr, createdBrokerCr.Name, defaultNamespace)
				CleanResource(createdPvc, pvc.Name, pvc.Namespace)
			}
		})
	})
})
