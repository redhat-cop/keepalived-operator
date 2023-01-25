package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	redhatcopv1alpha1 "github.com/redhat-cop/keepalived-operator/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var _ = Describe("keepalived controller", func() {

	const (
		KeepalivedGroupName      = "test-keepalived"
		KeepalivedGroupNamespace = "default"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	ctx := context.Background()
	keepalivedLookupKey := types.NamespacedName{Name: KeepalivedGroupName, Namespace: KeepalivedGroupNamespace}
	keepalivedConfigMap := &corev1.ConfigMap{}

	service1 := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      KeepalivedGroupName + "1",
			Namespace: KeepalivedGroupNamespace,
			Annotations: map[string]string{
				"keepalived-operator.redhat-cop.io/keepalivedgroup":     fmt.Sprintf("%s/%s", KeepalivedGroupNamespace, KeepalivedGroupName),
				"keepalived-operator.redhat-cop.io/updateservicestatus": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					Protocol:   "TCP",
					TargetPort: intstr.FromInt(6443),
				},
			},
			ExternalIPs: []string{
				"1.1.1.1",
			},
		},
	}
	service1LookupKey := types.NamespacedName{Name: KeepalivedGroupName + "1", Namespace: KeepalivedGroupNamespace}

	When("initialising a KeepalivedGroup and Service with updateservicestatus annotation", func() {
		keepalivedgroup := &redhatcopv1alpha1.KeepalivedGroup{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "redhatcop.redhat.io/v1alpha1",
				Kind:       "KeepalivedGroup",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      KeepalivedGroupName,
				Namespace: KeepalivedGroupNamespace,
			},
			Spec: redhatcopv1alpha1.KeepalivedGroupSpec{
				Image:              "registry.redhat.io/openshift4/ose-keepalived-ipfailover",
				Interface:          "eno1",
				BlacklistRouterIDs: []int{1, 2, 4, 5},
			},
		}
		It("should accept creation of a KeepalivedGroup and Service", func() {
			Expect(k8sClient.Create(ctx, keepalivedgroup)).Should(Succeed())
			Expect(k8sClient.Create(ctx, service1)).Should(Succeed())
		})
		It("should create a configmap with the router ID and IP", func() {
			Eventually(func() bool {
				return k8sClient.Get(ctx, keepalivedLookupKey, keepalivedConfigMap) == nil
			}, timeout, interval).Should(BeTrue())
			Expect(keepalivedConfigMap.Data["keepalived.conf"]).Should(ContainSubstring(service1.Spec.ExternalIPs[0]))
			Expect(keepalivedConfigMap.Data["keepalived.conf"]).Should(ContainSubstring("virtual_router_id 3"))
		})
		It("should create a daemonset", func() {
			service1DaemonSet := &appsv1.DaemonSet{}
			Eventually(func() bool {
				return k8sClient.Get(ctx, keepalivedLookupKey, service1DaemonSet) == nil
			}, timeout, interval).Should(BeTrue())
		})
		It("should set the LoadBalancer Ingress ip in the status of the Service", func() {
			service1result := &corev1.Service{}
			Eventually(func() string {
				if k8sClient.Get(ctx, service1LookupKey, service1result) == nil {
					if len(service1result.Status.LoadBalancer.Ingress) == 1 {
						return service1result.Status.LoadBalancer.Ingress[0].IP
					}
				}
				return ""
			}, timeout, interval).Should(Equal(service1.Spec.ExternalIPs[0]))
		})
	})

	When("changing a Service with updateservicestatus annotation", func() {
		service1NewIP := &corev1.Service{}
		service1result := &corev1.Service{}
		It("should allow updating the service with a new IP", func() {
			Expect(k8sClient.Get(ctx, service1LookupKey, service1NewIP)).Should(Succeed())
			service1NewIP.Spec.ExternalIPs[0] = "2.2.2.2"
			Expect(k8sClient.Update(ctx, service1NewIP)).Should(Succeed())
		})
		It("should update the IP in the configmap", func() {
			Eventually(func() string {
				if k8sClient.Get(ctx, keepalivedLookupKey, keepalivedConfigMap) == nil {
					if configData, ok := keepalivedConfigMap.Data["keepalived.conf"]; ok {
						return configData
					}
				}
				return ""
			}, timeout, interval).Should(ContainSubstring(service1NewIP.Spec.ExternalIPs[0]))
		})
		It("should update the LoadBalancer Ingress IP in the status of the Service", func() {
			Eventually(func() string {
				if k8sClient.Get(ctx, service1LookupKey, service1result) == nil {
					if len(service1result.Status.LoadBalancer.Ingress) == 1 {
						return service1result.Status.LoadBalancer.Ingress[0].IP
					}
				}
				return ""
			}, timeout, interval).Should(Equal(service1NewIP.Spec.ExternalIPs[0]))
		})
		It("should allow reverting back to original IP", func() {
			Expect(k8sClient.Get(ctx, service1LookupKey, service1NewIP)).Should(Succeed())
			service1NewIP.Spec.ExternalIPs[0] = service1.Spec.ExternalIPs[0]
			Expect(k8sClient.Update(ctx, service1NewIP)).Should(Succeed())
			Eventually(func() string {
				if k8sClient.Get(ctx, service1LookupKey, service1result) == nil {
					if len(service1result.Status.LoadBalancer.Ingress) == 1 {
						return service1result.Status.LoadBalancer.Ingress[0].IP
					}
				}
				return ""
			}, timeout, interval).Should(Equal(service1.Spec.ExternalIPs[0]))
		})
	})

	When("creating a second service without updateservicestatus annotation", func() {
		service2 := &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "Service",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      KeepalivedGroupName + "2",
				Namespace: KeepalivedGroupNamespace,
				Annotations: map[string]string{
					"keepalived-operator.redhat-cop.io/keepalivedgroup": fmt.Sprintf("%s/%s", KeepalivedGroupNamespace, KeepalivedGroupName),
				},
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{
					{
						Name:       "https",
						Port:       443,
						Protocol:   "TCP",
						TargetPort: intstr.FromInt(6443),
					},
				},
				ExternalIPs: []string{
					"1.1.1.2",
				},
			},
		}
		It("should allow creating the service", func() {
			Expect(k8sClient.Create(ctx, service2)).Should(Succeed())
		})
		It("should update the configmap with the new router ID and IP", func() {
			Eventually(func() string {
				if k8sClient.Get(ctx, keepalivedLookupKey, keepalivedConfigMap) == nil {
					if configData, ok := keepalivedConfigMap.Data["keepalived.conf"]; ok {
						return configData
					}
				}
				return ""
			}, timeout, interval).Should(And(ContainSubstring("virtual_router_id 6"), ContainSubstring("1.1.1.2")))
		})
	})

	When("removing a service", func() {
		It("should allow removal of the service", func() {
			Expect(k8sClient.Delete(ctx, service1)).Should(Succeed())
		})
		It("should remove the router ID and IP from the configmap", func() {
			Eventually(func() string {
				if k8sClient.Get(ctx, keepalivedLookupKey, keepalivedConfigMap) == nil {
					if configData, ok := keepalivedConfigMap.Data["keepalived.conf"]; ok {
						return configData
					}
				}
				return ""
			}, timeout, interval).Should(And(Not(ContainSubstring("virtual_router_id 3")), Not(ContainSubstring(service1.Spec.ExternalIPs[0])), ContainSubstring("virtual_router_id 6")))
		})
		When("adding another service", func() {
			service3 := &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      KeepalivedGroupName + "3",
					Namespace: KeepalivedGroupNamespace,
					Annotations: map[string]string{
						"keepalived-operator.redhat-cop.io/keepalivedgroup": fmt.Sprintf("%s/%s", KeepalivedGroupNamespace, KeepalivedGroupName),
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Ports: []corev1.ServicePort{
						{
							Name:       "https",
							Port:       443,
							Protocol:   "TCP",
							TargetPort: intstr.FromInt(6443),
						},
					},
					ExternalIPs: []string{
						"1.1.1.3",
					},
				},
			}
			It("should allow creating of the service", func() {
				Expect(k8sClient.Create(ctx, service3)).Should(Succeed())
			})
			It("Should reuse the removed services router ID", func() {
				Eventually(func() string {
					if k8sClient.Get(ctx, keepalivedLookupKey, keepalivedConfigMap) == nil {
						if configData, ok := keepalivedConfigMap.Data["keepalived.conf"]; ok {
							return configData
						}
					}
					return ""
				}, timeout, interval).Should(And(ContainSubstring("virtual_router_id 3"), ContainSubstring("virtual_router_id 6"), ContainSubstring(service3.Spec.ExternalIPs[0])))
			})
		})
	})

	//TODO: test multiple ExternalIPs are updated in the LoadBalancer status when using the updateservicestatus annotation.
})
