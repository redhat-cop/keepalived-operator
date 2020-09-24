package keepalivedgroup

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"text/template"

	redhatcopv1alpha1 "github.com/redhat-cop/keepalived-operator/pkg/apis/redhatcop/v1alpha1"
	"github.com/redhat-cop/operator-utils/pkg/util"
	"github.com/redhat-cop/operator-utils/pkg/util/apis"
	"github.com/scylladb/go-set/iset"
	"github.com/scylladb/go-set/strset"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
	podMonitorAPIVersion                    = "monitoring.coreos.com/v1"
	podMonitorKind                          = "PodMonitor"
)

var log = logf.Log.WithName(controllerName)
var keepalivedTemplate *template.Template
var supportsPodMonitors = "false"

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new KeepalivedGroup Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	var err error
	keepalivedTemplate, err = initializeTemplate()
	if err != nil {
		log.Error(err, "unable to initialize job template")
		return err
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {

	reconcilerBase := util.NewReconcilerBase(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig(), mgr.GetEventRecorderFor(controllerName))

	discoveryClient, err := reconcilerBase.GetDiscoveryClient()

	if err != nil {
		log.Error(err, "failed to initialize discovery client")
		return nil
	}

	resources, resourcesErr := discoveryClient.ServerResourcesForGroupVersion(podMonitorAPIVersion)

	if resourcesErr != nil {
		log.Error(err, "failed to discover resources")
		return nil
	}

	for _, apiResource := range resources.APIResources {
		if apiResource.Kind == podMonitorKind {
			supportsPodMonitors = "true"
			break
		}
	}

	return &ReconcileKeepalivedGroup{
		ReconcilerBase: reconcilerBase,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource KeepalivedGroup
	err = c.Watch(&source.Kind{Type: &redhatcopv1alpha1.KeepalivedGroup{
		TypeMeta: metav1.TypeMeta{
			Kind: "KeepalivedGroup",
		},
	}}, &handler.EnqueueRequestForObject{}, util.ResourceGenerationOrFinalizerChangedPredicate{})
	if err != nil {
		return err
	}

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

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Route
	err = c.Watch(&source.Kind{Type: &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
	}}, &enqueueRequestForReferredKeepAlivedGroup{
		Client: mgr.GetClient(),
	}, isAnnotatedService)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileKeepalivedGroup implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileKeepalivedGroup{}

// ReconcileKeepalivedGroup reconciles a KeepalivedGroup object
type ReconcileKeepalivedGroup struct {
	util.ReconcilerBase
}

// Reconcile reads that state of the cluster for a KeepalivedGroup object and makes changes based on the state read
// and what is in the KeepalivedGroup.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileKeepalivedGroup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling KeepalivedGroup")

	// Fetch the KeepalivedGroup instance
	instance := &redhatcopv1alpha1.KeepalivedGroup{}
	err := r.GetClient().Get(context.TODO(), request.NamespacedName, instance)
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
		return r.ManageError(instance, err)
	}

	if ok := r.IsInitialized(instance); !ok {
		err := r.GetClient().Update(context.TODO(), instance)
		if err != nil {
			log.Error(err, "unable to update instance", "instance", instance)
			return r.ManageError(instance, err)
		}
		return reconcile.Result{}, nil
	}

	services, err := r.getReferencingServices(instance)
	if err != nil {
		log.Error(err, "unable to get referencing services from", "instance", instance)
		return r.ManageError(instance, err)
	}
	_, err = assignRouterIDs(instance, services)
	if err != nil {
		log.Error(err, "unable assign router ids to", "instance", instance, "from services", services)
		return r.ManageError(instance, err)
	}
	objs, err := r.processTemplate(instance, services)
	if err != nil {
		log.Error(err, "unable process keepalived template from", "instance", instance, "and from services", services)
		return r.ManageError(instance, err)
	}
	for _, obj := range *objs {
		err = r.CreateOrUpdateResource(instance, instance.GetNamespace(), &obj)
		if err != nil {
			log.Error(err, "unable to create or update resource", "resource", obj)
			return r.ManageError(instance, err)
		}
	}
	return r.ManageSuccess(instance)
}

func assignRouterIDs(instance *redhatcopv1alpha1.KeepalivedGroup, services []corev1.Service) (bool, error) {
	assignedServices := []string{}
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
		assignedServices = append(assignedServices, key)
	}
	lbServices := []string{}
	for _, service := range services {
		lbServices = append(lbServices, apis.GetKeyShort(&service))
	}
	assignedServicesSet := strset.New(assignedServices...)
	lbServicesSet := strset.New(lbServices...)
	toBeRemovedSet := strset.Difference(assignedServicesSet, lbServicesSet)
	toBeAddedSet := strset.Difference(lbServicesSet, assignedServicesSet)

	for _, value := range toBeRemovedSet.List() {
		delete(instance.Status.RouterIDs, value)
	}
	for _, value := range instance.Status.RouterIDs {
		assignedIDs = append(assignedIDs, value)
	}
	// remove potential duplicates and sort
	assignedIDs = iset.New(assignedIDs...).List()
	sort.Ints(assignedIDs)
	if instance.Status.RouterIDs == nil {
		instance.Status.RouterIDs = map[string]int{}
	}
	for _, value := range toBeAddedSet.List() {
		id, err := findNextAvailableID(assignedIDs)
		if err != nil {
			log.Error(err, "unable assign a router id to", "service", value)
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
	sort.Ints(ids)
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

func (r *ReconcileKeepalivedGroup) processTemplate(instance *redhatcopv1alpha1.KeepalivedGroup, services []corev1.Service) (*[]unstructured.Unstructured, error) {
	imagename, ok := os.LookupEnv(imageNameEnv)
	if !ok {
		imagename = "quay.io/redhat-cop/keepalived-operator:latest"
	}
	objs, err := util.ProcessTemplateArray(struct {
		KeepalivedGroup *redhatcopv1alpha1.KeepalivedGroup
		Services        []corev1.Service
		Misc            map[string]string
	}{
		instance,
		services,
		map[string]string{
			"image":              imagename,
			"supportsPodMonitor": supportsPodMonitors,
		},
	}, keepalivedTemplate)
	if err != nil {
		log.Error(err, "unable to process template")
		return &[]unstructured.Unstructured{}, err
	}
	return &objs, nil
}

func (r *ReconcileKeepalivedGroup) getReferencingServices(instance *redhatcopv1alpha1.KeepalivedGroup) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	err := r.GetClient().List(context.TODO(), serviceList, &client.ListOptions{})
	if err != nil {
		log.Error(err, "unable to get list of load balancer services")
		return corev1.ServiceList{}.Items, err
	}
	//filter the returned list
	result := []corev1.Service{}
	for _, service := range serviceList.Items {
		value, ok := service.GetAnnotations()[keepalivedGroupAnnotation]
		if ok && (service.Spec.Type == corev1.ServiceTypeLoadBalancer || len(service.Spec.ExternalIPs) > 0) {
			namespacedName, err := getNamespacedName(value)
			if err != nil {
				log.Error(err, "unable to create namespaced name from ", "service", apis.GetKeyShort(&service), "annotation", keepalivedGroupAnnotation, "value", value)
				continue
			}
			if namespacedName.Name == instance.GetName() && namespacedName.Namespace == instance.GetNamespace() {
				result = append(result, service)
			}
		}
	}
	return result, nil
}

func (r *ReconcileKeepalivedGroup) IsInitialized(instance *redhatcopv1alpha1.KeepalivedGroup) bool {
	initialized := true
	if instance.Spec.Image == "" {
		instance.Spec.Image = "registry.redhat.io/openshift4/ose-keepalived-ipfailover"
		initialized = false
	}
	return initialized
}

func initializeTemplate() (*template.Template, error) {
	templateFileName, ok := os.LookupEnv(templateFileNameEnv)
	if !ok {
		templateFileName = "/etc/templates/job.template.yaml"
	}
	text, err := ioutil.ReadFile(templateFileName)
	if err != nil {
		log.Error(err, "Error reading job template file", "filename", templateFileName)
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
				log.Error(err, "unable to unmarshal json ", "string", jsonstr)
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
	}).Parse(string(text))
	if err != nil {
		log.Error(err, "Error parsing template", "template", string(text))
		return &template.Template{}, err
	}
	return jobTemplate, err
}

type enqueueRequestForReferredKeepAlivedGroup struct {
	client.Client
}

func (e *enqueueRequestForReferredKeepAlivedGroup) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	keepalivedGroup, ok := evt.Meta.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			log.Error(err, "unable to create namespaced name from", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
			return
		}
		q.Add(reconcile.Request{NamespacedName: namespaced})
	}
}

func (e *enqueueRequestForReferredKeepAlivedGroup) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	keepalivedGroup, ok := evt.MetaNew.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			log.Info(err.Error(), "unable to create namespaced name from MetaNew", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
		} else {
			q.Add(reconcile.Request{NamespacedName: namespaced})
		}
	}
	keepalivedGroup, ok = evt.MetaOld.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			log.Info(err.Error(), "unable to create namespaced name from MetaOld", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
		} else {
			q.Add(reconcile.Request{NamespacedName: namespaced})
		}
	}
}

// Delete implements EventHandler
func (e *enqueueRequestForReferredKeepAlivedGroup) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	keepalivedGroup, ok := evt.Meta.GetAnnotations()[keepalivedGroupAnnotation]
	if ok {
		namespaced, err := getNamespacedName(keepalivedGroup)
		if err != nil {
			log.Error(err, "unable to create namespaced name from", "annotation", keepalivedGroupAnnotation, "value", keepalivedGroup)
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
