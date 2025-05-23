package controllers

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var _ = Describe("Cluster Reconciler", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		clusterName  = "cluster-test-cluster"
		namespace    = clusterName
		timeout      = time.Second * 30
		interval     = time.Second * 1
		consistently = time.Second * 10
	)
	var (
		OpensearchCluster      = ComposeOpensearchCrd(clusterName, namespace)
		service                = corev1.Service{}
		preUpgradeStatusLength int
	)

	/// ------- Creation Check phase -------

	When("Creating a OpenSearch CRD instance", func() {
		It("Should create the namespace first", func() {
			Expect(CreateNamespace(k8sClient, &OpensearchCluster)).Should(Succeed())
			By("Create cluster ns ")
			Eventually(func() bool {
				return IsNsCreated(k8sClient, namespace)
			}, timeout, interval).Should(BeTrue())
		})

		It("should apply the cluster instance successfully", func() {
			Expect(k8sClient.Create(context.Background(), &OpensearchCluster)).Should(Succeed())
		})

	})

	/// ------- Tests logic Check phase -------

	When("Creating a OpenSearchCluster kind Instance", func() {
		It("should create a new opensearch cluster ", func() {
			Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: OpensearchCluster.Namespace, Name: OpensearchCluster.Spec.General.ServiceName}, &service); err != nil {
					return false
				}
				for _, nodePoolSpec := range OpensearchCluster.Spec.NodePools {
					nodePool := appsv1.StatefulSet{}
					if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: OpensearchCluster.Namespace, Name: fmt.Sprintf("%s-%s", OpensearchCluster.Spec.General.ServiceName, nodePoolSpec.Component)}, &service); err != nil {
						return false
					}
					if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: OpensearchCluster.Namespace, Name: clusterName + "-" + nodePoolSpec.Component}, &nodePool); err != nil {
						return false
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())
		})

		It("should configure statefulsets correctly", func() {
			wg := sync.WaitGroup{}
			for _, nodePool := range OpensearchCluster.Spec.NodePools {
				wg.Add(1)
				By(fmt.Sprintf("checking %s nodepool initial master", nodePool.Component))
				go func(nodePool opsterv1.NodePool) {
					defer GinkgoRecover()
					defer wg.Done()
					sts := &appsv1.StatefulSet{}
					Eventually(func() []corev1.EnvVar {
						if err := k8sClient.Get(context.Background(), types.NamespacedName{
							Namespace: OpensearchCluster.Namespace,
							Name:      clusterName + "-" + nodePool.Component,
						}, sts); err != nil {
							return []corev1.EnvVar{}
						}
						return sts.Spec.Template.Spec.Containers[0].Env
					}, timeout, interval).Should(ContainElement(corev1.EnvVar{
						Name:  "foo",
						Value: "bar",
					}))
					Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("500m"))
					Expect(sts.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("2Gi"))
				}(nodePool)
			}
			wg.Wait()
		})

		It("should set nodepool specific config", func() {
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-client", OpensearchCluster.Name),
					Namespace: OpensearchCluster.Namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  "baz",
				Value: "bat",
			}))
		})

		It("should create a bootstrap pod", func() {
			bootstrapName := fmt.Sprintf("%s-bootstrap-0", OpensearchCluster.Name)
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      bootstrapName,
					Namespace: OpensearchCluster.Namespace,
				}, &corev1.Pod{})
			}, timeout, interval).Should(Succeed())
			wg := sync.WaitGroup{}
			for _, nodePool := range OpensearchCluster.Spec.NodePools {
				wg.Add(1)
				By(fmt.Sprintf("checking %s nodepool initial master", nodePool.Component))
				go func(nodePool opsterv1.NodePool) {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() []corev1.EnvVar {
						sts := &appsv1.StatefulSet{}
						if err := k8sClient.Get(context.Background(), types.NamespacedName{
							Namespace: OpensearchCluster.Namespace,
							Name:      clusterName + "-" + nodePool.Component,
						}, sts); err != nil {
							return []corev1.EnvVar{}
						}
						return sts.Spec.Template.Spec.Containers[0].Env
					}, timeout, interval).Should(ContainElement(corev1.EnvVar{
						Name:  "cluster.initial_master_nodes",
						Value: bootstrapName,
					}))
				}(nodePool)
			}
			wg.Wait()
		})
		It("should create a discovery service", func() {
			discoveryName := fmt.Sprintf("%s-discovery", OpensearchCluster.Name)
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      discoveryName,
					Namespace: OpensearchCluster.Namespace,
				}, &corev1.Service{})
			}, timeout, interval).Should(Succeed())
			wg := sync.WaitGroup{}
			for _, nodePool := range OpensearchCluster.Spec.NodePools {
				wg.Add(1)
				By(fmt.Sprintf("checking %s nodepool initial master", nodePool.Component))
				go func(nodePool opsterv1.NodePool) {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() []corev1.EnvVar {
						sts := &appsv1.StatefulSet{}
						if err := k8sClient.Get(context.Background(), types.NamespacedName{
							Namespace: OpensearchCluster.Namespace,
							Name:      clusterName + "-" + nodePool.Component,
						}, sts); err != nil {
							return []corev1.EnvVar{}
						}
						return sts.Spec.Template.Spec.Containers[0].Env
					}, timeout, interval).Should(ContainElement(corev1.EnvVar{
						Name:  "discovery.seed_hosts",
						Value: discoveryName,
					}))
				}(nodePool)
			}
			wg.Wait()
		})
		It("should set correct owner references", func() {
			service := corev1.Service{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: OpensearchCluster.Spec.General.ServiceName}, &service)).To(Succeed())
			Expect(HasOwnerReference(&service, &OpensearchCluster)).To(BeTrue())
			for _, nodePoolSpec := range OpensearchCluster.Spec.NodePools {
				nodePool := appsv1.StatefulSet{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName + "-" + nodePoolSpec.Component}, &nodePool)).To(Succeed())
				Expect(HasOwnerReference(&nodePool, &OpensearchCluster)).To(BeTrue())
				Expect(k8sClient.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: OpensearchCluster.Spec.General.ServiceName + "-" + nodePoolSpec.Component}, &service)).To(Succeed())
				Expect(HasOwnerReference(&service, &OpensearchCluster)).To(BeTrue())
			}
		})
		It("should set the version status", func() {
			Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster); err != nil {
					return false
				}
				return OpensearchCluster.Status.Version == "1.0.0"
			}, timeout, interval).Should(BeTrue())
		})
	})

	/// ------- Tests nodepool cleanup -------
	When("Updating an OpensearchCluster kind instance", func() {
		It("should remove old node pools", func() {
			// Fetch the latest version of the opensearch object
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster)).Should(Succeed())

			// Update the opensearch object
			OpensearchCluster.Spec.NodePools = OpensearchCluster.Spec.NodePools[:2]
			OpensearchCluster.Spec.General.Version = "1.1.0"
			Expect(k8sClient.Update(context.Background(), &OpensearchCluster)).Should(Succeed())

			Eventually(func() bool {
				stsList := &appsv1.StatefulSetList{}
				err := k8sClient.List(context.Background(), stsList, client.InNamespace(OpensearchCluster.Name))
				if err != nil {
					return false
				}

				return len(stsList.Items) == 2
			})
		})
		It("should not update the node pool image version", func() {
			Consistently(func() bool {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster); err != nil {
					return false
				}
				return OpensearchCluster.Status.Version == "1.0.0"
			}, consistently, interval).Should(BeTrue())
			wg := sync.WaitGroup{}
			for _, pool := range OpensearchCluster.Spec.NodePools {
				wg.Add(1)
				By(fmt.Sprintf("checking %s node pool", pool.Component))
				go func(pool opsterv1.NodePool) {
					defer GinkgoRecover()
					defer wg.Done()

					sts := &appsv1.StatefulSet{}

					Consistently(func() bool {
						if err := k8sClient.Get(
							context.Background(),
							client.ObjectKey{
								Namespace: OpensearchCluster.Namespace,
								Name:      clusterName + "-" + pool.Component,
							}, sts); err != nil {
							return false
						}
						return sts.Spec.Template.Spec.Containers[0].Image == "docker.io/opensearchproject/opensearch:1.0.0"
					}, consistently, interval).Should(BeTrue())
				}(pool)
			}
			wg.Wait()
		})
	})
	When("A node pool is upgrading", func() {
		Specify("updating the status should succeed", func() {
			status := opsterv1.ComponentStatus{
				Component:   "Upgrader",
				Description: "nodes",
				Status:      "Upgrading",
			}
			Expect(func() error {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster); err != nil {
					return err
				}
				preUpgradeStatusLength = len(OpensearchCluster.Status.ComponentsStatus)
				OpensearchCluster.Status.ComponentsStatus = append(OpensearchCluster.Status.ComponentsStatus, status)
				return k8sClient.Status().Update(context.Background(), &OpensearchCluster)
			}()).To(Succeed())
		})
		It("should update the node pool image", func() {
			Eventually(func() bool {
				sts := &appsv1.StatefulSet{}
				if err := k8sClient.Get(
					context.Background(),
					client.ObjectKey{
						Namespace: OpensearchCluster.Namespace,
						Name:      clusterName + "-nodes",
					}, sts); err != nil {
					return false
				}
				return sts.Spec.Template.Spec.Containers[0].Image == "docker.io/opensearchproject/opensearch:1.1.0"
			}, timeout, interval).Should(BeTrue())
		})
	})
	When("a cluster is upgraded", func() {
		Specify("updating the status should succeed", func() {
			currentStatus := opsterv1.ComponentStatus{
				Component:   "Upgrader",
				Status:      "Upgrading",
				Description: "nodes",
			}
			componentStatus := opsterv1.ComponentStatus{
				Component:   "Upgrader",
				Status:      "Upgraded",
				Description: "nodes",
			}
			masterComponentStatus := opsterv1.ComponentStatus{
				Component:   "Upgrader",
				Status:      "Upgraded",
				Description: "master",
			}
			Expect(func() error {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster); err != nil {
					return err
				}
				OpensearchCluster.Status.ComponentsStatus = helpers.Replace(currentStatus, componentStatus, OpensearchCluster.Status.ComponentsStatus)
				OpensearchCluster.Status.ComponentsStatus = append(OpensearchCluster.Status.ComponentsStatus, masterComponentStatus)
				return k8sClient.Status().Update(context.Background(), &OpensearchCluster)
			}()).To(Succeed())
		})
		It("should cleanup the status", func() {
			Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster); err != nil {
					return false
				}
				return len(OpensearchCluster.Status.ComponentsStatus) == preUpgradeStatusLength
			}, timeout, interval)
			Eventually(func() bool {
				if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&OpensearchCluster), &OpensearchCluster); err != nil {
					return false
				}
				return OpensearchCluster.Status.Version == "1.1.0"
			}, timeout, interval)
		})
		It("should update all the node pools", func() {
			wg := sync.WaitGroup{}
			for _, nodePool := range OpensearchCluster.Spec.NodePools {
				wg.Add(1)
				go func(nodePool opsterv1.NodePool) {
					defer GinkgoRecover()
					defer wg.Done()
					Eventually(func() bool {
						sts := &appsv1.StatefulSet{}
						if err := k8sClient.Get(context.Background(), types.NamespacedName{
							Namespace: OpensearchCluster.Namespace,
							Name:      clusterName + "-" + nodePool.Component,
						}, sts); err != nil {
							return false
						}
						return sts.Spec.Template.Spec.Containers[0].Image == "docker.io/opensearchproject/opensearch:1.1.0"
					}, timeout, interval).Should(BeTrue())
				}(nodePool)
			}
			wg.Wait()
		})
	})
})
