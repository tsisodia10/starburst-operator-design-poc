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

package addon

import (
	"context"
	"fmt"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/example/starburst-addon-operator/api/v1alpha1"
	validator "github.com/example/starburst-addon-operator/pkg/webhook"

	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StarburstAddonReconciler reconciles a StarburstAddon object
type StarburstAddonReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=example.com.example.com,resources=starburstaddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=example.com.example.com,resources=starburstaddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=example.com.example.com,resources=starburstaddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=example.com.example.com,resources=starburstenterprises,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=example.com.example.com,resources=starburstenterprises/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=example.com.example.com,resources=starburstenterprises/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
func (r *StarburstAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// fetch subject addon
	addon := &v1alpha1.StarburstAddon{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, addon); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("addon not found, probably deleted")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "failed to fetch addon")
		return ctrl.Result{}, err
	}

	finalizerName := "starburstaddons.example.com/finalizer"

	// cleanup for deletion
	if !addon.ObjectMeta.DeletionTimestamp.IsZero() {
		// object is currently being deleted
		if controllerutil.ContainsFinalizer(addon, finalizerName) {
			// finalizer exists, delete child enterprise
			enterpriseDelete := &unstructured.Unstructured{}
			enterpriseDelete.SetGroupVersionKind(validator.EnterpriseGvk)

			if err := r.Client.Get(
				ctx,
				types.NamespacedName{
					Namespace: req.Namespace,
					Name:      createDesiredEnterpriseName(req.Name),
				},
				enterpriseDelete); err == nil {
				// found enterprise resource, delete it
				if err := r.Client.Delete(ctx, enterpriseDelete); err != nil {
					if k8serrors.IsInternalError(err) && strings.Contains(err.Error(), "connection refused") {
						// if the webhooks's cert secret is not yes fully propagated as files,
						// we might encounter a connection refused scenario, therefore requeueing
						logger.Info("failed deleting the enterprise cr, webhook validation refused connection, requeueing")
						return ctrl.Result{Requeue: true}, nil
					}
					logger.Error(err, "failed deleting enterprise cr")
					return ctrl.Result{}, err
				}
			}

			// deletion done, remove finalizer
			controllerutil.RemoveFinalizer(addon, finalizerName)
			if err := r.Client.Update(ctx, addon); err != nil {
				logger.Error(err, "failed to remove finalizer from new addon")
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	// object is NOT currently being deleted, add finalizer
	if !controllerutil.ContainsFinalizer(addon, finalizerName) {
		controllerutil.AddFinalizer(addon, finalizerName)
	}
	if err := r.Client.Update(ctx, addon); err != nil {
		logger.Error(err, "failed to set finalizer for new addon")
		return ctrl.Result{}, err
	}

	// NOTE secrets, configmaps, prometheus servers, and service monitors from the original operator will goes here

	// load enterprise manifest from path propagated by a secret created for the vault keys
	manifest, err := os.ReadFile("/opt/enterprise/enterprise.yaml")
	if err != nil {
		logger.Error(err, "failed to fetch enterprise manifest")
		return ctrl.Result{}, err
	}

	// reconcile unstructured enterprise resource
	desiredEnterprise, err := createDesiredEnterprise(addon, manifest)
	if err != nil {
		logger.Error(err, "failed to fetch enterprise manifest")
		return ctrl.Result{}, err
	}

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(desiredEnterprise.GroupVersionKind())

	// fetch the existing enterprise resource
	if err := r.Client.Get(ctx, req.NamespacedName, current); err != nil {
		if k8serrors.IsNotFound(err) {
			// if current enterprise not found, create a new one
			if err := r.Client.Create(ctx, &desiredEnterprise); err != nil {
				if k8serrors.IsInternalError(err) && strings.Contains(err.Error(), "connection refused") {
					// if the webhooks's cert secret is not yes fully propagated as files,
					// we might encounter a connection refused scenario, therefore requeueing
					logger.Info("failed creating enterprise cr, webhook validation refused connection, requeueing")
					return ctrl.Result{Requeue: true}, nil
				}
				if k8serrors.IsAlreadyExists(err) {
					logger.Info("enterprise cr was created recently")
					return ctrl.Result{}, nil
				}
				logger.Error(err, "failed creating enterprise cr")
				return ctrl.Result{}, err
			}
		}

		// if current enterprise exists but fetching it failed
		logger.Error(err, "failed creating enterprise cr.")
		return ctrl.Result{}, err
	}

	// reconcile back to desired if current was changed
	if !equality.Semantic.DeepDerivative(desiredEnterprise, current) {
		// NOTE equality should be based on business logic
		if err := r.Client.Update(ctx, &desiredEnterprise); err != nil {
			logger.Error(err, "failed reconciling enterprise cr, requeuing")
			return ctrl.Result{Requeue: true}, err
		}
	}

	return ctrl.Result{}, nil
}

// sets up the controller with the manager
func (r *StarburstAddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.StarburstAddon{}).
		Complete(r)
}

// create the desired enterprise resource
func createDesiredEnterprise(addon *v1alpha1.StarburstAddon, manifest []byte) (unstructured.Unstructured, error) {
	enterprise := unstructured.Unstructured{}
	deserialized := make(map[string]interface{})
	if err := yaml.Unmarshal(manifest, deserialized); err != nil {
		return enterprise, err
	}

	enterprise.SetUnstructuredContent(deserialized)
	enterprise.SetGroupVersionKind(validator.EnterpriseGvk)
	enterprise.SetName(createDesiredEnterpriseName(addon.Name))
	enterprise.SetNamespace(addon.Namespace)
	enterprise.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(addon, addon.GetObjectKind().GroupVersionKind()),
	})

	return enterprise, nil
}

// create the desired enterprise name
func createDesiredEnterpriseName(name string) string {
	return fmt.Sprintf("%s-enterprise", name)
}
