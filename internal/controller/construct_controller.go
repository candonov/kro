/*
Copyright 2023.

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

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/symphony/api/v1alpha1"
	xv1alpha1 "github.com/aws/symphony/api/v1alpha1"
	"github.com/aws/symphony/internal/crd"
	"github.com/aws/symphony/internal/dynamiccontroller"
	"github.com/aws/symphony/internal/finalizer"
	openapischema "github.com/aws/symphony/internal/schema"
)

// ConstructReconciler reconciles a Construct object
type ConstructReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	CRDManager        crd.Manager
	OpenAPISchema     *openapischema.OpenAPISchemaTransformer
	DynamicController *dynamiccontroller.DynamicController
}

//+kubebuilder:rbac:groups=x.symphony.k8s.aws,resources=constructs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=x.symphony.k8s.aws,resources=constructs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=x.symphony.k8s.aws,resources=constructs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Construct object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ConstructReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := klog.FromContext(ctx)
	log.Info("Reconciling", "resource", req.NamespacedName)

	var construct v1alpha1.Construct
	err := r.Get(ctx, req.NamespacedName, &construct)
	if err != nil {
		log.Error(err, "unable to fetch Construct object")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Got construct from the api server", "name", req.NamespacedName)

	log.Info("Transforming construct definition to OpenAPIv3 schema", "name", req.NamespacedName)

	// Handle creation
	oaSchema, err := r.OpenAPISchema.Transform(construct.Spec.Definition.Spec.Raw)
	if err != nil {
		log.Info("unable to transform OpenAPI schema")
		return ctrl.Result{}, err
	}

	/* 	yamlSchema, err := yaml.Marshal(oaSchema)
	   	if err != nil {
	   		log.Info("unable to marshal OpenAPI schema")
	   		return ctrl.Result{}, err
	   	}
	   	fmt.Println(string(yamlSchema)) */

	customRD := crd.FromOpenAPIV3Schema(construct.Spec.ApiVersion, construct.Spec.Kind, oaSchema)

	/* 	bb, err := yaml.Marshal(customRD)
	   	if err != nil {
	   		log.Info("unable to marshal OpenAPI schema")
	   		return ctrl.Result{}, err
	   	}
	   	fmt.Println(string(bb)) */

	log.Info("Creating custom resource definition", "crd_name", customRD.Name)
	err = r.CRDManager.Ensure(ctx, customRD)
	if err != nil {
		log.Info("unable to ensure CRD")
		return ctrl.Result{}, err
	}

	gvr := schema.GroupVersionResource{
		Group:    customRD.Spec.Group,
		Version:  customRD.Spec.Versions[0].Name,
		Resource: customRD.Spec.Names.Plural,
	}

	// Handle deletions
	if !construct.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("construct is deleted")
		log.Info("Unregistering GVK in symphony's dynamic controller", "crd_name", customRD.Name, "gvr", gvr)
		r.DynamicController.UnregisterGVK(gvr)
		log.Info("Unregistering workflow operator in symphony's dynamic controller", "crd_name", customRD.Name, "gvr", gvr)
		r.DynamicController.UnregisterWorkflowOperator(gvr)
		log.Info("Removing finalizer from construct", "crd_name", customRD.Name, "gvr", gvr)
		err = r.setUnmanaged(ctx, &construct)
		if err != nil {
			log.Info("unable to set unmanaged")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	gvrStr := fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
	log.Info("Registering GVK in symphony's dynamic controller", "crd_name", customRD.Name, "gvr", gvrStr)
	r.DynamicController.SafeRegisterGVK(gvr)

	log.Info("Registering workflow operator in symphony's dynamic controller", "crd_name", customRD.Name, "gvr", gvrStr)
	err = r.DynamicController.RegisterWorkflowOperator(
		ctx,
		gvr,
		&construct,
	)
	if err != nil {
		log.Info("unable to register workflow operator")
		return ctrl.Result{}, err
	}

	// Set managed
	log.Info("Setting symphony finalizers", "crd_name", customRD.Name, "gvr", gvrStr)
	err = r.setManaged(ctx, &construct)
	if err != nil {
		log.Info("unable to set managed")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConstructReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&xv1alpha1.Construct{}).
		Complete(r)
}

func (r *ConstructReconciler) setManaged(ctx context.Context, construct *v1alpha1.Construct) error {
	newFinalizers := finalizer.AddSymphonyFinalizer(construct)
	dc := construct.DeepCopy()
	dc.Finalizers = newFinalizers
	return r.Update(ctx, dc)
}

func (r *ConstructReconciler) setUnmanaged(ctx context.Context, construct *v1alpha1.Construct) error {
	newFinalizers := finalizer.RemoveSymphonyFinalizer(construct)
	dc := construct.DeepCopy()
	dc.Finalizers = newFinalizers
	return r.Update(ctx, dc)
}