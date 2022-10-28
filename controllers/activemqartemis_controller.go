/*
Copyright 2021.

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
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	rtclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/artemiscloud/activemq-artemis-operator/pkg/resources"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/namer"
	"github.com/pkg/errors"

	brokerv1beta1 "github.com/artemiscloud/activemq-artemis-operator/api/v1beta1"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/common"
	"github.com/artemiscloud/activemq-artemis-operator/pkg/utils/selectors"
)

var clog = ctrl.Log.WithName("controller_v1beta1activemqartemis")

var namespaceToConfigHandler = make(map[types.NamespacedName]common.ActiveMQArtemisConfigHandler)

func GetBrokerConfigHandler(brokerNamespacedName types.NamespacedName) (handler common.ActiveMQArtemisConfigHandler) {
	for _, handler := range namespaceToConfigHandler {
		if handler.IsApplicableFor(brokerNamespacedName) {
			return handler
		}
	}
	return nil
}

func (r *ActiveMQArtemisReconciler) UpdatePodForSecurity(securityHandlerNamespacedName types.NamespacedName, handler common.ActiveMQArtemisConfigHandler) error {

	existingCrs := &brokerv1beta1.ActiveMQArtemisList{}
	var err error
	opts := &rtclient.ListOptions{}
	if err = r.Client.List(context.TODO(), existingCrs, opts); err == nil {
		var candidate types.NamespacedName
		for index, artemis := range existingCrs.Items {
			candidate.Name = artemis.Name
			candidate.Namespace = artemis.Namespace
			if handler.IsApplicableFor(candidate) {
				clog.Info("force reconcile for security", "handler", securityHandlerNamespacedName, "CR", candidate)
				r.events <- event.GenericEvent{Object: &existingCrs.Items[index]}
			}
		}
	}
	return err
}

func (r *ActiveMQArtemisReconciler) RemoveBrokerConfigHandler(namespacedName types.NamespacedName) {
	clog.Info("Removing config handler", "name", namespacedName)
	oldHandler, ok := namespaceToConfigHandler[namespacedName]
	if ok {
		delete(namespaceToConfigHandler, namespacedName)
		clog.Info("Handler removed", "name", namespacedName)
		r.UpdatePodForSecurity(namespacedName, oldHandler)
	}
}

func (r *ActiveMQArtemisReconciler) AddBrokerConfigHandler(namespacedName types.NamespacedName, handler common.ActiveMQArtemisConfigHandler, toReconcile bool) error {
	if _, ok := namespaceToConfigHandler[namespacedName]; ok {
		clog.V(1).Info("There is an old config handler, it'll be replaced")
	}
	namespaceToConfigHandler[namespacedName] = handler
	clog.V(1).Info("A new config handler has been added", "handler", handler)
	if toReconcile {
		clog.V(1).Info("Updating broker security")
		return r.UpdatePodForSecurity(namespacedName, handler)
	}
	return nil
}

// ActiveMQArtemisReconciler reconciles a ActiveMQArtemis object
type ActiveMQArtemisReconciler struct {
	rtclient.Client
	Scheme *runtime.Scheme
	events chan event.GenericEvent
}

//run 'make manifests' after changing the following rbac markers

//+kubebuilder:rbac:groups=broker.amq.io,namespace=activemq-artemis-operator,resources=activemqartemises,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=broker.amq.io,namespace=activemq-artemis-operator,resources=activemqartemises/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=broker.amq.io,namespace=activemq-artemis-operator,resources=activemqartemises/finalizers,verbs=update
//+kubebuilder:rbac:groups=broker.amq.io,namespace=activemq-artemis-operator,resources=pods,verbs=get;list
//+kubebuilder:rbac:groups="",namespace=activemq-artemis-operator,resources=pods;services;endpoints;persistentvolumeclaims;events;configmaps;secrets;routes;serviceaccounts,verbs=*
//+kubebuilder:rbac:groups="",namespace=activemq-artemis-operator,resources=namespaces,verbs=get
//+kubebuilder:rbac:groups=apps,namespace=activemq-artemis-operator,resources=deployments;daemonsets;replicasets;statefulsets,verbs=*
//+kubebuilder:rbac:groups=networking.k8s.io,namespace=activemq-artemis-operator,resources=ingresses,verbs=get;list;watch;create;delete
//+kubebuilder:rbac:groups=route.openshift.io,namespace=activemq-artemis-operator,resources=routes;routes/custom-host;routes/status,verbs=get;list;watch;create;delete;update
//+kubebuilder:rbac:groups=monitoring.coreos.com,namespace=activemq-artemis-operator,resources=servicemonitors,verbs=get;create
//+kubebuilder:rbac:groups=apps,namespace=activemq-artemis-operator,resources=deployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace=activemq-artemis-operator,resources=roles;rolebindings,verbs=create;get;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ActiveMQArtemis object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ActiveMQArtemisReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	reqLogger := ctrl.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name, "Reconciling", "ActiveMQArtemis")

	customResource := &brokerv1beta1.ActiveMQArtemis{}

	// Fetch the ActiveMQArtemis instance
	// When first creating this will have err == nil
	// When deleting after creation this will have err NotFound
	// When deleting before creation reconcile won't be called
	err := r.Get(context.TODO(), request.NamespacedName, customResource)

	if err != nil {
		if apierrors.IsNotFound(err) {
			reqLogger.Info("ActiveMQArtemis Controller Reconcile encountered a IsNotFound, for request NamespacedName " + request.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		reqLogger.Error(err, "unable to retrieve the ActiveMQArtemis", "request", request)
		return ctrl.Result{}, err
	}

	namer := MakeNamers(customResource)
	reconciler := ActiveMQArtemisReconcilerImpl{}

	result := ctrl.Result{}

	if hasValidationErrors, err := validate(customResource, r.Client, r.Scheme); !hasValidationErrors && err == nil {
		requeue := reconciler.Process(customResource, *namer, r.Client, r.Scheme)

		err = UpdatePodStatus(customResource, r.Client, request.NamespacedName)
		if err != nil {
			reqLogger.Error(err, "unable to update pod status", "Request Namespace", request.Namespace, "Request Name", request.Name)
			return ctrl.Result{RequeueAfter: common.GetReconcileResyncPeriod()}, err
		}

		result = UpdateBrokerPropertiesStatus(customResource, r.Client, r.Scheme)
		if requeue {
			result = ctrl.Result{RequeueAfter: common.GetReconcileResyncPeriod()}
		}
	}

	err = UpdateCRStatus(customResource, r.Client, request.NamespacedName)

	if err != nil {
		if apierrors.IsConflict(err) {
			reqLogger.V(1).Info("unable to update ActiveMQArtemis status", "Request Namespace", request.Namespace, "Request Name", request.Name, "error", err)
			err = nil // we don't want the controller event loop reporting this as an error
		} else {
			reqLogger.Error(err, "unable to update ActiveMQArtemis status", "Request Namespace", request.Namespace, "Request Name", request.Name)
		}
		return ctrl.Result{RequeueAfter: common.GetReconcileResyncPeriod()}, err
	}

	if result.IsZero() {
		reqLogger.Info("resource successfully reconciled")
	} else {
		reqLogger.Info("requeue resource")
	}
	return result, err
}

func validate(customResource *brokerv1beta1.ActiveMQArtemis, client rtclient.Client, scheme *runtime.Scheme) (bool, error) {
	// Do additional validation here
	validationCondition := metav1.Condition{
		Type:   common.ValidConditionType,
		Status: metav1.ConditionTrue,
		Reason: common.ValidConditionSuccessReason,
	}
	condition, err := validateExtraMounts(customResource, client, scheme)
	if err != nil {
		return false, err
	}
	if condition != nil {
		validationCondition = *condition
	}

	meta.SetStatusCondition(&customResource.Status.Conditions, validationCondition)
	return false, nil
}

func validateExtraMounts(customResource *brokerv1beta1.ActiveMQArtemis, client rtclient.Client, scheme *runtime.Scheme) (*metav1.Condition, error) {
	for _, cm := range customResource.Spec.DeploymentPlan.ExtraMounts.ConfigMaps {
		found, err := validateExtraMount(cm, customResource.Namespace, &corev1.ConfigMap{}, client, scheme)
		if err != nil {
			return nil, err
		}
		if !found {
			return &metav1.Condition{
				Type:    common.ValidConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  common.ValidConditionMissingResourcesReason,
				Message: fmt.Sprintf("Missing required configMap %v", cm),
			}, nil
		}
	}
	for _, s := range customResource.Spec.DeploymentPlan.ExtraMounts.Secrets {
		found, err := validateExtraMount(s, customResource.Namespace, &corev1.Secret{}, client, scheme)
		if err != nil {
			return nil, err
		}
		if !found {
			return &metav1.Condition{
				Type:    common.ValidConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  common.ValidConditionMissingResourcesReason,
				Message: fmt.Sprintf("Missing required secret %v", s),
			}, nil
		}
	}
	return nil, nil
}

func validateExtraMount(name, namespace string, obj rtclient.Object, client rtclient.Client, scheme *runtime.Scheme) (bool, error) {
	err := client.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		} else {
			return false, err
		}
	}
	return true, nil
}

type Namers struct {
	SsGlobalName                  string
	SsNameBuilder                 namer.NamerData
	SvcHeadlessNameBuilder        namer.NamerData
	SvcPingNameBuilder            namer.NamerData
	PodsNameBuilder               namer.NamerData
	SecretsCredentialsNameBuilder namer.NamerData
	SecretsConsoleNameBuilder     namer.NamerData
	SecretsNettyNameBuilder       namer.NamerData
	LabelBuilder                  selectors.LabelerData
	GLOBAL_DATA_PATH              string
}

func MakeNamers(customResource *brokerv1beta1.ActiveMQArtemis) *Namers {
	newNamers := Namers{
		SsGlobalName:                  "",
		SsNameBuilder:                 namer.NamerData{},
		SvcHeadlessNameBuilder:        namer.NamerData{},
		SvcPingNameBuilder:            namer.NamerData{},
		PodsNameBuilder:               namer.NamerData{},
		SecretsCredentialsNameBuilder: namer.NamerData{},
		SecretsConsoleNameBuilder:     namer.NamerData{},
		SecretsNettyNameBuilder:       namer.NamerData{},
		LabelBuilder:                  selectors.LabelerData{},
		GLOBAL_DATA_PATH:              "/opt/" + customResource.Name + "/data",
	}
	newNamers.SsNameBuilder.Base(customResource.Name).Suffix("ss").Generate()
	newNamers.SsGlobalName = customResource.Name
	newNamers.SvcHeadlessNameBuilder.Prefix(customResource.Name).Base("hdls").Suffix("svc").Generate()
	newNamers.SvcPingNameBuilder.Prefix(customResource.Name).Base("ping").Suffix("svc").Generate()
	newNamers.PodsNameBuilder.Base(customResource.Name).Suffix("container").Generate()
	newNamers.SecretsCredentialsNameBuilder.Prefix(customResource.Name).Base("credentials").Suffix("secret").Generate()
	newNamers.SecretsConsoleNameBuilder.Prefix(customResource.Name).Base("console").Suffix("secret").Generate()
	newNamers.SecretsNettyNameBuilder.Prefix(customResource.Name).Base("netty").Suffix("secret").Generate()
	newNamers.LabelBuilder.Base(customResource.Name).Suffix("app").Generate()

	return &newNamers
}

func GetDefaultLabels(cr *brokerv1beta1.ActiveMQArtemis) map[string]string {
	defaultLabelData := selectors.LabelerData{}
	defaultLabelData.Base(cr.Name).Suffix("app").Generate()
	return defaultLabelData.Labels()
}

//only test uses this
func NewReconcileActiveMQArtemis(c rtclient.Client, s *runtime.Scheme) ActiveMQArtemisReconciler {
	return ActiveMQArtemisReconciler{
		Client: c,
		Scheme: s,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ActiveMQArtemisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&brokerv1beta1.ActiveMQArtemis{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Pod{})

	var err error
	controller, err := builder.Build(r)
	if err == nil {
		r.events = make(chan event.GenericEvent)
		err = controller.Watch(
			&source.Channel{Source: r.events},
			&handler.EnqueueRequestForObject{},
		)
	}
	return err
}

func UpdateCRStatus(cr *brokerv1beta1.ActiveMQArtemis, client rtclient.Client, namespacedName types.NamespacedName) error {

	common.SetReadyCondition(&cr.Status.Conditions)

	current := &brokerv1beta1.ActiveMQArtemis{}

	err := client.Get(context.TODO(), namespacedName, current)
	if err != nil {
		clog.Error(err, "unable to retrieve current resource", "ActiveMQArtemis", namespacedName)
		return err
	}

	if !reflect.DeepEqual(current.Status.PodStatus, cr.Status.PodStatus) {
		return resources.UpdateStatus(client, cr)
	}
	if len(current.Status.Conditions) != len(cr.Status.Conditions) {
		return resources.UpdateStatus(client, cr)
	}
	for _, c := range current.Status.Conditions {
		if !common.IsConditionPresentAndEqual(cr.Status.Conditions, c) {
			return resources.UpdateStatus(client, cr)
		}
	}

	return nil
}

// Controller Errors

type ArtemisError interface {
	Error() string
	Requeue() bool
}

type unknownJolokiaError struct {
	cause error
}
type jolokiaClientNotFoundError struct {
	cause error
}

const StatusOutOfSyncError statusOutOfSyncError = "BrokerProperties status out of sync"

type statusOutOfSyncError string

func NewUnknownJolokiaError(err error) unknownJolokiaError {
	return unknownJolokiaError{
		err,
	}
}

func (e unknownJolokiaError) Error() string {
	return e.cause.Error()
}

func (e unknownJolokiaError) Requeue() bool {
	return false
}

func NewJolokiaClientsNotFoundError(err error) jolokiaClientNotFoundError {
	return jolokiaClientNotFoundError{
		err,
	}
}

func (e jolokiaClientNotFoundError) Error() string {
	return errors.Wrap(e.cause, "no available Jolokia Clients found").Error()
}

func (e jolokiaClientNotFoundError) Requeue() bool {
	return true
}

func (e statusOutOfSyncError) Error() string {
	return string(StatusOutOfSyncError)
}

func (e statusOutOfSyncError) Requeue() bool {
	return true
}
