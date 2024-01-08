package resources

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Create(owner v1.Object, client client.Client, scheme *runtime.Scheme, clientObject client.Object) error {
	reqLogger := ctrl.Log.WithName("k8s_actions").WithValues("ActiveMQArtemis Name", clientObject.GetName(), "Namespace", clientObject.GetNamespace())
	objectTypeString := reflect.TypeOf(clientObject.(runtime.Object)).String()
	reqLogger.V(1).Info("Creating new " + objectTypeString)

	SetOwnerAndController(owner, clientObject)

	var err error
	if err = client.Create(context.TODO(), clientObject); err != nil {
		// Add error detail for use later
		reqLogger.Error(err, "Failed to create new "+objectTypeString)
	} else {
		reqLogger.V(1).Info("Created new " + objectTypeString)
	}

	if err != nil {
		err = fmt.Errorf("failed to create new %s, %v", objectTypeString, err)
	}
	return err
}

func SetOwnerAndController(owner v1.Object, clientObject client.Object) {
	reqLogger := ctrl.Log.WithName("k8s_actions").WithValues("ActiveMQArtemis Name", clientObject.GetName(), "Namespace", clientObject.GetNamespace())

	gvk := owner.(runtime.Object).GetObjectKind().GroupVersionKind()
	isController := true
	ref := v1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		UID:        owner.GetUID(),
		Name:       owner.GetName(),
		Controller: &isController, // ControllerManager.Owns watches match on Controller=true
	}
	clientObject.SetOwnerReferences([]v1.OwnerReference{ref})
	reqLogger.V(1).Info("set owner-controller reference", "target", clientObject.GetObjectKind().GroupVersionKind().String(), "owner", ref)
}

func Retrieve(namespacedName types.NamespacedName, client client.Client, objectDefinition client.Object) error {
	reqLogger := ctrl.Log.WithName("k8s_actions").WithValues("ActiveMQArtemis Name", namespacedName.Name)
	objectTypeString := reflect.TypeOf(objectDefinition.(runtime.Object)).String()
	reqLogger.V(1).Info("Retrieving " + objectTypeString)

	return client.Get(context.TODO(), namespacedName, objectDefinition)
}

func Update(client client.Client, clientObject client.Object) error {

	reqLogger := ctrl.Log.WithName("k8s_actions").WithValues("ActiveMQArtemis Name", clientObject.GetName(), "Namespace", clientObject.GetNamespace())
	objectTypeString := reflect.TypeOf(clientObject.(runtime.Object)).String()
	reqLogger.V(1).Info("Updating "+objectTypeString, "obj", clientObject)

	var err error = nil
	if err = client.Update(context.TODO(), clientObject); err != nil {
		switch statusError := err.(type) {
		case *errors.StatusError:
			if statusError.ErrStatus.Status == v1.StatusFailure &&
				statusError.ErrStatus.Code == http.StatusUnprocessableEntity &&
				statusError.ErrStatus.Reason == v1.StatusReasonInvalid {

				// "StatefulSet.apps is invalid: spec: Forbidden: updates to statefulset spec for fields other than 'replicas', 'template', 'updateStrategy' and 'minReadySeconds' are forbidden"}
				reqLogger.V(1).Info("Deleting on failed updating "+objectTypeString, "obj", clientObject, "Forbidden", err)
				err = Delete(client, clientObject)
			} else if statusError.ErrStatus.Status == v1.StatusFailure &&
				statusError.ErrStatus.Code == http.StatusConflict &&
				statusError.ErrStatus.Reason == v1.StatusReasonConflict {
				// we can live with conflict and will retry, no need to log
				err = fmt.Errorf("failed to update %s due to conflict", objectTypeString)
			} else {
				reqLogger.Error(err, "got error on update", "resourceVersion", clientObject.GetResourceVersion())
			}
		default:
			reqLogger.Error(err, "Failed to update "+objectTypeString)
		}
	}

	if err != nil {
		err = fmt.Errorf("failed to update %s, %v", objectTypeString, err)
	}
	return err
}

func UpdateStatus(client client.Client, clientObject client.Object) error {
	reqLogger := ctrl.Log.WithName("k8s_actions").WithValues("ActiveMQArtemis Name", clientObject.GetName(), "Namespace", clientObject.GetNamespace())
	objectTypeString := reflect.TypeOf(clientObject.(runtime.Object)).String()
	reqLogger.V(1).Info("Updating status "+objectTypeString, "obj", clientObject)

	var err error = nil
	if err = client.Status().Update(context.TODO(), clientObject); err != nil {
		if errors.IsConflict(err) {
			reqLogger.V(1).Info("Failed to update status on "+objectTypeString, "error", err)
		} else {
			reqLogger.Error(err, "Failed to update status on "+objectTypeString)
		}
	}
	return err
}

func Delete(client client.Client, clientObject client.Object) error {

	reqLogger := ctrl.Log.WithName("k8s_actions").WithValues("ActiveMQArtemis Name", clientObject.GetName(), "Namespace", clientObject.GetNamespace())
	objectTypeString := reflect.TypeOf(clientObject.(runtime.Object)).String()
	reqLogger.V(2).Info("Deleting " + objectTypeString)

	var err error = nil
	if err = client.Delete(context.TODO(), clientObject); err != nil {
		reqLogger.Error(err, "Failed to delete "+objectTypeString)
	}

	if err != nil {
		err = fmt.Errorf("failed to delete %s, %v", objectTypeString, err)
	}

	return err
}
