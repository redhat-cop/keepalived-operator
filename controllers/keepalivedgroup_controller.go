/*
Copyright 2020.

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

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	redhatcopv1alpha1 "github.com/redhat-cop/keepalived-operator/api/v1alpha1"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/operator-utils/pkg/util/apis"
	"github.com/redhat-cop/operator-utils/pkg/util/templates"
	"github.com/scylladb/go-set/iset"
	"github.com/scylladb/go-set/strset"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName                          = "keepalived-controller"
	templateFileNameEnv                     = "KEEPALIVEDGROUP_TEMPLATE_FILE_NAME"
	imageNameEnv                            = "KEEPALIVED_OPERATOR_IMAGE_NAME"
	keepalivedGroupAnnotation               = "keepalived-operator.redhat-cop.io/keepalivedgroup"
	keepalivedGroupVerbatimConfigAnnotation = "keepalived-operator.redhat-cop.io/verbatimconfig"
	keepalivedSpreadVIPsAnnotation          = "keepalived-operator.redhat-cop.io/spreadvips"
	keepalivedGroupLabel                    = "keepalivedGroup"
	podMonitorAPIVersion                    = "monitoring.coreos.com/v1"
	podMonitorKind                          = "PodMonitor"
)

// KeepalivedGroupReconciler reconciles a KeepalivedGroup object
type KeepalivedGroupReconciler struct {
	util.ReconcilerBase
	Log                 logr.Logger
	supportsPodMonitors string
	keepalivedTemplate  *template.Template
}

func (r *KeepalivedGroupReconciler) setSupportForPodMonitorAvailable() {
	r.supportsPodMonitors = "false"
	discoveryClient, err := r.GetDiscoveryClient()

	if err != nil {
		r.Log.Error(err, "failed to initialize discovery client")
		return
	}

	resources, resourcesErr := discoveryClient.ServerResourcesForGroupVersion(podMonitorAPIVersion)

	if resourcesErr != nil {
		r.Log.Error(err, "failed to discover resources")
		return
	}

	for _, apiResource := range resources.APIResources {
		if apiResource.Kind == podMonitorKind {
			r.supportsPodMonitors = "true"
			break
		}
	}
}

// +kubebuilder:rbac:groups=redhatcop.redhat.io,resources=keepalivedgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redhatcop.redhat.io,resources=keepalivedgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redhatcop.redhat.io,resources=keepalivedgroups/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services;endpoints;pods;secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="apps",resources=daemonsets;daemonsets/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="monitoring.coreos.com",resources=podmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="monitoring.coreos.com",resources=podmonitors/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KeepalivedGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *KeepalivedGroupReconciler) Reconcile(context context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("keepalivedgroup", req.NamespacedName)

	// Fetch the KeepalivedGroup instance
	instance := &redhatcopv1alpha1.KeepalivedGroup{}
	err := r.GetClient().Get(context, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if ok, err := r.IsValid(instance); !ok {
		return r.ManageError(context, instance, err)
	}

	if ok := r.IsInitialized(instance); !ok {
		err := r.GetClient().Update(context, instance)
		if err != nil {
			log.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(context, instance, err)
		}
		return reconcile.Result{}, nil
	}

	// Check if VRRP authentication is needed and if so extract credentials
	authPass := ""
	if instance.Spec.PasswordAuth.SecretRef.Name != "" {
		secret := &corev1.Secret{}
		err := r.GetClient().Get(context, types.NamespacedName{Namespace: instance.GetNamespace(), Name: instance.Spec.PasswordAuth.SecretRef.Name}, secret)
		if err != nil {
			// Requeue and log error
			log.Error(err, "could not find passwordAuth secret", "instance", instance)
			return r.ManageError(context, instance, err)
		}
		pass, ok := secret.Data[instance.Spec.PasswordAuth.SecretKey]
		if !ok {
			// Requeue and log error
			err = fmt.Errorf("could not find key %s in secret %s in namespace %s", instance.Spec.PasswordAuth.SecretKey, instance.Spec.PasswordAuth.SecretRef.Name, instance.GetNamespace())
			log.Error(err, "could not find referenced key in passwordAuth secret", "instance", instance)
			return r.ManageError(context, instance, err)
		}
		authPass = string(pass)
	}

	pods, err := r.getKeepalivedPods(instance)
	services, err := r.getReferencingServices(instance)
	if err != nil {
		log.Error(err, "unable to get referencing services from", "instance", instance)
		return r.ManageError(context, instance, err)
	}
	_, err = r.assignRouterIDs(instance, services)
	if err != nil {
		log.Error(err, "unable assign router ids to", "instance", instance, "from services", services)
		return r.ManageError(context, instance, err)
	}
	objs, err := r.processTemplate(context, instance, services, pods, authPass)
	if err != nil {
		log.Error(err, "unable process keepalived template from", "instance", instance, "and from services", services)
		return r.ManageError(context, instance, err)
	}

	// this code needs to be commented until this bug is resolved: https://github.com/kubernetes-sigs/yaml/issues/47
	// lockedResources := []lockedresource.LockedResource{}
	// for _, obj := range *objs {
	// 	lockedResource := lockedresource.LockedResource{
	// 		Unstructured: obj,
	// 	}
	// 	lockedResources = append(lockedResources, lockedResource)
	// }

	// err = r.UpdateLockedResources(context, instance, lockedResources, []lockedpatch.LockedPatch{})
	// if err != nil {
	// 	log.Error(err, "unable to update locked resources")
	// 	return r.ManageError(context, instance, err)
	// }

	// this code needs to stay here until this bug is resolved: https://github.com/kubernetes-sigs/yaml/issues/47
	for _, obj := range *objs {
		err = r.CreateOrUpdateResource(context, instance, instance.GetNamespace(), &obj)
		if err != nil {
			log.Error(err, "unable to create or update resource", "resource", obj)
			return r.ManageError(context, instance, err)
		}
	}
	return r.ManageSuccess(context, instance)
}

func (r *KeepalivedGroupReconciler) assignRouterIDs(instance *redhatcopv1alpha1.KeepalivedGroup, services []corev1.Service) (bool, error) {
	assignedInstances := []string{}
	assignedIDs := []int{}
	if len(instance.Spec.BlacklistRouterIDs) > 0 {
		assignedIDs = append(assignedIDs, instance.Spec.BlacklistRouterIDs...)
		for key, val := range instance.Status.RouterIDs {
			for _, id := range instance.Spec.BlacklistRouterIDs {
				if val == id {
					delete(instance.Status.RouterIDs, key)
					break
				}
			}
		}
	}
	for key := range instance.Status.RouterIDs {
		assignedInstances = append(assignedInstances, key)
	}
	vrrpInstances := servicesToVRRPInstances(services)

	assignedInstancesSet := strset.New(assignedInstances...)
	vrrpInstancesSet := strset.New(vrrpInstances...)
	toBeRemovedSet := strset.Difference(assignedInstancesSet, vrrpInstancesSet)
	toBeAddedSet := strset.Difference(vrrpInstancesSet, assignedInstancesSet)

	for _, value := range toBeRemovedSet.List() {
		delete(instance.Status.RouterIDs, value)
	}
	for _, value := range instance.Status.RouterIDs {
		assignedIDs = append(assignedIDs, value)
	}
	// remove potential duplicates and sort
	assignedIDs = iset.New(assignedIDs...).List()
	if instance.Status.RouterIDs == nil {
		instance.Status.RouterIDs = map[string]int{}
	}
	for _, value := range toBeAddedSet.List() {
		id, err := findNextAvailableID(assignedIDs)
		if err != nil {
			r.Log.Error(err, "unable assign a router id to", "service", value)
			return false, err
		}
		instance.Status.RouterIDs[value] = id
		assignedIDs = append(assignedIDs, instance.Status.RouterIDs[value])
	}
	return (toBeAddedSet.Size() > 0 || toBeRemovedSet.Size() > 0), nil
}

func findNextAvailableID(ids []int) (int, error) {
	if len(ids) == 0 {
		return 1, nil
	}
	usedSet := iset.New(ids...)
	for i := 1; i <= 255; i++ {
		used := false
		if usedSet.Has(i) {
			used = true
		}
		if !used {
			return i, nil
		}
	}
	return 0, errors.New("cannot allocate more than 255 ids in one keepalived group")
}

func servicesToVRRPInstances(services []corev1.Service) []string {
	vrrpInstances := []string{}
	for _, service := range services {
		svcName := apis.GetKeyShort(&service)
		if ann, ok := service.GetAnnotations()[keepalivedSpreadVIPsAnnotation]; ok && ann == "true" {
			for _, ingress := range service.Status.LoadBalancer.Ingress {
				vrrpInstances = append(vrrpInstances, svcName+"/"+ingress.IP)
			}
			for _, ip := range service.Spec.ExternalIPs {
				vrrpInstances = append(vrrpInstances, svcName+"/"+ip)
			}
		} else {
			vrrpInstances = append(vrrpInstances, svcName)
		}
	}

	return vrrpInstances
}

func (r *KeepalivedGroupReconciler) processTemplate(ctx context.Context, instance *redhatcopv1alpha1.KeepalivedGroup, services []corev1.Service, pods []corev1.Pod, authPass string) (*[]unstructured.Unstructured, error) {
	// sort services and pods to ensure deterministic template output
	sort.SliceStable(services, func(i, j int) bool {
		if services[i].GetNamespace() == services[j].GetNamespace() {
			return services[i].GetName() < services[j].GetName()
		}
		return services[i].GetNamespace() < services[j].GetNamespace()
	})
	sort.SliceStable(pods, func(i, j int) bool {
		return pods[i].GetName() < pods[j].GetName()
	})

	imagename, ok := os.LookupEnv(imageNameEnv)
	if !ok {
		imagename = "quay.io/redhat-cop/keepalived-operator:latest"
	}
	objs, err := templates.ProcessTemplateArray(ctx, struct {
		KeepalivedGroup *redhatcopv1alpha1.KeepalivedGroup
		Services        []corev1.Service
		KeepalivedPods  []corev1.Pod
		Misc            map[string]string
	}{
		instance,
		services,
		pods,
		map[string]string{
			"image":              imagename,
			"supportsPodMonitor": r.supportsPodMonitors,
			"authPass":           authPass,
		},
	}, r.keepalivedTemplate)
	if err != nil {
		r.Log.Error(err, "unable to process template")
		return &[]unstructured.Unstructured{}, err
	}
	return &objs, nil
}

func (r *KeepalivedGroupReconciler) getKeepalivedPods(instance *redhatcopv1alpha1.KeepalivedGroup) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := r.GetClient().List(context.TODO(), podList, &client.ListOptions{Namespace: instance.GetNamespace(), LabelSelector: labels.SelectorFromSet(map[string]string{keepalivedGroupLabel: instance.GetName()})})
	if err != nil {
		r.Log.Error(err, "unable to get list of keepalived pods")
		return corev1.PodList{}.Items, err
	}
	return podList.Items, nil
}

func (r *KeepalivedGroupReconciler) getReferencingServices(instance *redhatcopv1alpha1.KeepalivedGroup) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	err := r.GetClient().List(context.TODO(), serviceList, &client.ListOptions{})
	if err != nil {
		r.Log.Error(err, "unable to get list of load balancer services")
		return corev1.ServiceList{}.Items, err
	}
	//filter the returned list
	result := []corev1.Service{}
	for _, service := range serviceList.Items {
		value, ok := service.GetAnnotations()[keepalivedGroupAnnotation]
		if ok && (service.Spec.Type == corev1.ServiceTypeLoadBalancer || len(service.Spec.ExternalIPs) > 0) {
			namespacedName, err := getNamespacedName(value)
			if err != nil {
				r.Log.Error(err, "unable to create namespaced name from ", "service", apis.GetKeyShort(&service), "annotation", keepalivedGroupAnnotation, "value", value)
				continue
			}
			if namespacedName.Name == instance.GetName() && namespacedName.Namespace == instance.GetNamespace() {
				result = append(result, service)
			}
		}
	}
	return result, nil
}

// func (r *KeepalivedGroupReconciler) IsInitialized(instance *redhatcopv1alpha1.KeepalivedGroup) bool {
// 	initialized := true
// 	if instance.Spec.Image == "" {
// 		instance.Spec.Image = "registry.redhat.io/openshift4/ose-keepalived-ipfailover"
// 		initialized = false
// 	}
// 	return initialized
// }

func (r *KeepalivedGroupReconciler) initializeTemplate() (*template.Template, error) {
	templateFileName, ok := os.LookupEnv(templateFileNameEnv)
	if !ok {
		templateFileName = "/etc/templates/job.template.yaml"
	}
	text, err := ioutil.ReadFile(templateFileName)
	if err != nil {
		r.Log.Error(err, "Error reading job template file", "filename", templateFileName)
		return &template.Template{}, err
	}
	jobTemplate, err := template.New("KeepalivedGroup").Funcs(template.FuncMap{
		"parseJson": func(jsonstr string) map[string]string {
			if jsonstr == "" {
				return map[string]string{}
			}
			var m map[string]string
			err := json.Unmarshal([]byte(jsonstr), &m)
			if err != nil {
				r.Log.Error(err, "unable to unmarshal json ", "string", jsonstr)
				return map[string]string{}
			}
			return m
		},
		"mergeStringSlices": func(lbis []corev1.LoadBalancerIngress, s2 []string) []string {
			var s1 = []string{}
			for _, lbi := range lbis {
				if lbi.IP != "" {
					s1 = append(s1, lbi.IP)
				}
			}
			return strset.Union(strset.New(s1...), strset.New(s2...)).List()
		},
		"modulus": func(a, b int) int { return a % b },
	}).Parse(string(text))
	if err != nil {
		r.Log.Error(err, "Error parsing template", "template", string(text))
		return &template.Template{}, err
	}
	return jobTemplate, err
}

type enqueueRequestForReferredKeepAlivedGroup struct {
	client.Client
	Log logr.Logger
}

func (e *enqueueRequestForReferredKeepAlivedGroup) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	keepalivedGroup, ok := evt.Object.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			e.Log.Error(err, "unable to create namespaced name from", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
			return
		}
		q.Add(reconcile.Request{NamespacedName: namespaced})
	}
}

func (e *enqueueRequestForReferredKeepAlivedGroup) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	keepalivedGroup, ok := evt.ObjectNew.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			e.Log.Info(err.Error(), "unable to create namespaced name from MetaNew", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
		} else {
			q.Add(reconcile.Request{NamespacedName: namespaced})
		}
	}
	keepalivedGroup, ok = evt.ObjectOld.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			e.Log.Info(err.Error(), "unable to create namespaced name from MetaOld", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
		} else {
			q.Add(reconcile.Request{NamespacedName: namespaced})
		}
	}
}

// Delete implements EventHandler
func (e *enqueueRequestForReferredKeepAlivedGroup) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	keepalivedGroup, ok := evt.Object.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			e.Log.Error(err, "unable to create namespaced name from", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
			return
		}
		q.Add(reconcile.Request{NamespacedName: namespaced})
	}
}

// Generic implements EventHandler
func (e *enqueueRequestForReferredKeepAlivedGroup) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	return
}

func getNamespacedName(namespaced string) (types.NamespacedName, error) {
	elements := strings.Split(namespaced, "/")
	if len(elements) != 2 {
		return types.NamespacedName{}, errors.New("unable to split string into name and namespace using '/' as separator: " + namespaced)
	}
	return types.NamespacedName{
		Name:      elements[1],
		Namespace: elements[0],
	}, nil
}

// Handler to issue reconciles for KeepalivedGroup resources based on changes on keepalived pods
func (r *KeepalivedGroupReconciler) requestsForKeepalivedPodChange(obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		r.Log.Error(fmt.Errorf("expected a Pod, got %T", pod), "could not process pod change")
		return nil
	}

	keepalivedGroup, ok := pod.GetLabels()[keepalivedGroupLabel]
	if !ok {
		r.Log.Error(fmt.Errorf("could not extract keepalivedGroup from keepalived pod %s in namespace %s", pod.GetName(), pod.GetNamespace()), "could not process pod change")
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: pod.GetNamespace(), Name: keepalivedGroup}}}
}

// PodChange is a predicate that filters Pod changes to issue KeepalivedGroup reconciles for creation and deletion of keepalived pods
type PodChange struct {
	predicate.Funcs
}

// Update filters out pod updates
func (PodChange) Update(e event.UpdateEvent) bool {
	return false
}

// Create filters out pod creations if they are not keepalived pods
func (PodChange) Create(e event.CreateEvent) bool {
	pod, ok := e.Object.(*corev1.Pod)
	if !ok {
		return false
	}
	if _, ok := pod.GetLabels()[keepalivedGroupLabel]; !ok {
		return false
	}
	return true
}

// Delete filters out pod deletions if they are not keepalived pods
func (PodChange) Delete(e event.DeleteEvent) bool {
	pod, ok := e.Object.(*corev1.Pod)
	if !ok {
		return false
	}
	if _, ok := pod.GetLabels()[keepalivedGroupLabel]; !ok {
		return false
	}
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *KeepalivedGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.setSupportForPodMonitorAvailable()
	keepalivedTemplate, err := r.initializeTemplate()
	if err != nil {
		r.Log.Error(err, "unable to initialize job template")
		return err
	}
	r.keepalivedTemplate = keepalivedTemplate
	// this will filter new secrets and secrets where the content changed
	// secret that are actually referenced by routes will be filtered by the handler
	isAnnotatedService := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			service, ok := e.ObjectNew.DeepCopyObject().(*corev1.Service)
			if ok {
				if _, ok := service.GetAnnotations()[keepalivedGroupAnnotation]; ok && (service.Spec.Type == corev1.ServiceTypeLoadBalancer || len(service.Spec.ExternalIPs) > 0) {
					return true
				}
			}
			service, ok = e.ObjectOld.DeepCopyObject().(*corev1.Service)
			if ok {
				if _, ok := service.GetAnnotations()[keepalivedGroupAnnotation]; ok && (service.Spec.Type == corev1.ServiceTypeLoadBalancer || len(service.Spec.ExternalIPs) > 0) {
					return true
				}
			}
			return false
		},
		CreateFunc: func(e event.CreateEvent) bool {
			service, ok := e.Object.DeepCopyObject().(*corev1.Service)
			if !ok {
				return false
			}
			if _, ok := service.GetAnnotations()[keepalivedGroupAnnotation]; ok && (service.Spec.Type == corev1.ServiceTypeLoadBalancer || len(service.Spec.ExternalIPs) > 0) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			service, ok := e.Object.DeepCopyObject().(*corev1.Service)
			if !ok {
				return false
			}
			if _, ok := service.GetAnnotations()[keepalivedGroupAnnotation]; ok && (service.Spec.Type == corev1.ServiceTypeLoadBalancer || len(service.Spec.ExternalIPs) > 0) {
				return true
			}
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&redhatcopv1alpha1.KeepalivedGroup{}, builder.WithPredicates(util.ResourceGenerationOrFinalizerChangedPredicate{})).
		Watches(&source.Kind{Type: &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind: "Service",
			},
		}}, &enqueueRequestForReferredKeepAlivedGroup{
			Client: mgr.GetClient(),
		}, builder.WithPredicates(isAnnotatedService)).
		Watches(&source.Kind{Type: &corev1.Pod{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForKeepalivedPodChange),
			builder.WithPredicates(PodChange{}),
		).
		Complete(r)
}
