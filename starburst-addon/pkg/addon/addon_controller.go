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

	"github.com/example/starburst-addon-operator/api/v1alpha1"

	yaml "gopkg.in/yaml.v3"

	promv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	promv2 "github.com/prometheus-operator/prometheus-operator/pkg/client/monitoring/v1"
	promv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/client/monitoring/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

	// build enterprise resource from file propagated by a secret created for the vault keys
	desiredEnterprise, err := buildEnterpriseResource(addon, "/opt/enterprise/enterprise.yaml")
	if err != nil {
		logger.Error(err, "failed to fetch enterprise manifest")
		return ctrl.Result{}, err
	}

	finalizerName := "starburstaddons.example.com/finalizer"

	// cleanup for deletion
	if !addon.ObjectMeta.DeletionTimestamp.IsZero() {
		// object is currently being deleted
		if controllerutil.ContainsFinalizer(addon, finalizerName) {
			// finalizer exists, delete child enterprise
			enterpriseDelete := &unstructured.Unstructured{}
			enterpriseDelete.SetGroupVersionKind(desiredEnterprise.GetObjectKind().GroupVersionKind())

			if err := r.Client.Get(
				ctx,
				types.NamespacedName{
					Namespace: req.Namespace,
					Name:      buildEnterpriseName(req.Name),
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

	// Secret
	vault := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      "starburst-addon",
		Namespace: req.Namespace,
	}, vault); err != nil {

		if k8serrors.IsNotFound(err) {
			logger.Info("Addon Secret not found.")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, fmt.Errorf("could not get Addon Secret: %v", err)
	}

	// Deploy Prometheus
	prometheus := &promv1.Prometheus{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, prometheus); err != nil && k8serrors.IsNotFound(err) {
		logger.Info("Prometheus not found. Creating...")
		// tokenURL, remoteWriteURL, clusterID string
		prometheus = r.DeployPrometheus(addon, vault, string(vault.Data["token-url"]), string(vault.Data["remote-write-url"]), string(vault.Data["regex"]), fetchClusterID())
		if err := r.Client.Create(ctx, prometheus); err != nil {
			logger.Error(err, "Could not create Prometheus")
			return ctrl.Result{Requeue: true}, fmt.Errorf("could not create Prometheus: %v", err)
		}

		// Prometheus created successfully
		// We will requeue the request to ensure the Prometheus is created
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "could not get Prometheus")
		// return the error for the next reconcile
		return ctrl.Result{Requeue: true}, fmt.Errorf("could not get Prometheus: %v", err)
	}

	// Deploy ServiceMonitor
	serviceMonitor := &promv1.ServiceMonitor{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, serviceMonitor); err != nil && k8serrors.IsNotFound(err) {
		logger.Info("Service Monitor not found. Creating...")
		serviceMonitor = r.DeployServiceMonitor(addon)
		if err := r.Client.Create(ctx, serviceMonitor); err != nil {
			logger.Error(err, "Could not create Service Monitor")
			return ctrl.Result{Requeue: true}, fmt.Errorf("could not create service monitor: %v", err)
		}
	}

	// Deploy Federation ServiceMonitor
	fedServiceMonitor := &promv1.ServiceMonitor{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name + "-federation",
		Namespace: req.Namespace,
	}, fedServiceMonitor); err != nil && k8serrors.IsNotFound(err) {
		logger.Info("Federation Service Monitor not found. Creating...")
		fedServiceMonitor = r.DeployFederationServiceMonitor(addon, string(vault.Data["metrics"]))
		if err := r.Client.Create(ctx, fedServiceMonitor); err != nil {
			logger.Error(err, "Could not create Federation Service Monitor")
			return ctrl.Result{Requeue: true}, fmt.Errorf("could not create federation service monitor: %v", err)
		}
	}

	// Deploy PrometheusRules
	prometheusRule := &promv1.PrometheusRule{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	}, prometheusRule); err != nil && k8serrors.IsNotFound(err) {
		logger.Info("Prometheus Rules not found. Creating...")
		prometheusRule = r.DeployPrometheusRules(addon, string(vault.Data["rules"]))
		if err := r.Client.Create(ctx, prometheusRule); err != nil {
			logger.Error(err, "Could not create Prometheus Rules")
			return ctrl.Result{Requeue: true}, fmt.Errorf("could not create Prometheus Rules: %v", err)
		}
	}

	// reconcile unstructured enterprise resource
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

// builds enterprise resource from file setting the addon as the owner
func buildEnterpriseResource(addon *v1alpha1.StarburstAddon, file string) (unstructured.Unstructured, error) {
	enterprise := unstructured.Unstructured{}
	// load enterprise manifest from file
	manifest, err := os.ReadFile(file)
	if err != nil {
		return enterprise, err
	}
	// deserialize the loaded manifest to a yaml
	deserialized := make(map[string]interface{})
	if err := yaml.Unmarshal(manifest, deserialized); err != nil {
		return enterprise, err
	}
	// set unstructured data from the deserialized yaml file
	enterprise.SetUnstructuredContent(deserialized)
	// set name and owner refs
	enterprise.SetName(buildEnterpriseName(addon.Name))
	enterprise.SetNamespace(addon.Namespace)
	enterprise.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(addon, addon.GetObjectKind().GroupVersionKind()),
	})

	return enterprise, nil
}

func (r *StarburstAddonReconciler) DeployServiceMonitor(addon *v1alpha1.StarburstAddon) *promv1.ServiceMonitor {
	return &promv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name + "-servicemonitor",
			Namespace: addon.Namespace,
		},
		Spec: promv1.ServiceMonitorSpec{
			NamespaceSelector: promv1.NamespaceSelector{
				MatchNames: []string{addon.Namespace},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": addon.Name + "-enterprise", //AppName is same as addon.Name - refer how starburst enterprise CR is created in buildEntepriseRespurce(), appName == SB CR name
				},
			},
			Endpoints: []promv1.Endpoint{
				{
					Port:     "metrics",
					Interval: "2s",
				},
			},
		},
	}
}

func (r *StarburstAddonReconciler) DeployPrometheusRules(addon *v1alpha1.StarburstAddon, rules string) *promv1.PrometheusRule {
	unstructuredRule := unstructured.Unstructured{}

	// deserialize the loaded manifest to a yaml
	deserializedRules := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(rules), deserializedRules); err != nil {
		fmt.Printf("Error in umarshalling to yaml")
	}

	// set unstructured data from the deserialized yaml file
	unstructuredRule.SetUnstructuredContent(deserializedRules)

	rule := promv2.PrometheusRulesFromUnstructured(unstructuredRule)

	// create PrometheusRules step by step
	promRules := &promv1.PrometheusRule{}
	promRules.APIVersion = "monitoring.coreos.com/v1"
	promRules.Kind = "PrometheusRule"
	promRules.Name = addon.Name + "-rules"
	promRules.Namespace = addon.Namespace
	promRules.Spec = *&promv1.PrometheusRuleSpec{
		Groups: []promv1.RuleGroup{
			{
				Name:  "starburst_alert_rules",
				Rules: rule, //promRules is a slice of promv1.Rule
			},
		},
	}
	return promRules
}

func (r *StarburstAddonReconciler) DeployFederationServiceMonitor(addon *v1alpha1.StarburstAddon, metrics string) *promv1.ServiceMonitor {

	unstructuredMetrics := unstructured.Unstructured{}

	// deserialize the loaded manifest to a yaml
	deserializedMetrics := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(metrics), deserializedMetrics); err != nil {
		fmt.Printf("Error in umarshalling to yaml")
	}

	// set unstructured data from the deserialized yaml file
	unstructuredMetrics.SetUnstructuredContent(deserializedMetrics)

	// create metrics format from unstructuredMetrics
	metric := promv1alpha1.ServiceMonitorFromUnstructured(unstructuredMetrics)

	// create federated serviceMonitor
	fedServiceMonitor := &promv1.ServiceMonitor{}
	fedServiceMonitor.APIVersion = "monitoring.coreos.com/v1"
	fedServiceMonitor.Kind = "ServiceMonitor"
	fedServiceMonitor.Name = addon.Name + "-federation"
	fedServiceMonitor.Namespace = addon.Namespace
	fedServiceMonitor.Spec = *&promv1.ServiceMonitorSpec{
		JobLabel: "openshift-monitoring-federation",
		NamespaceSelector: promv1.NamespaceSelector{
			MatchNames: []string{
				"openshift-monitoring",
			},
		},
		Selector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app.kubernetes.io/instance": "k8s",
			},
		},
		Endpoints: []promv1.Endpoint{
			{
				BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				Port:            "web",
				Path:            "/federate",
				Interval:        "30s",
				Scheme:          "https",
				Params:          metric,
				TLSConfig: &promv1.TLSConfig{
					SafeTLSConfig: promv1.SafeTLSConfig{
						InsecureSkipVerify: true,
						ServerName:         "prometheus-k8s.openshift-monitoring.svc.cluster.local",
					},
					CAFile: "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt",
				},
			},
		},
	}

	return fedServiceMonitor
}

func (r *StarburstAddonReconciler) DeployPrometheus(addon *v1alpha1.StarburstAddon, vault *corev1.Secret, tokenURL, remoteWriteURL, regex, clusterID string) *promv1.Prometheus {

	prometheusName := addon.Name + "-prometheus"
	vaultSecretName := vault.Name
	//prometheusSelector := addon.Spec.ResourceSelector

	return &promv1.Prometheus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      addon.Name + "-prometheus",
			Namespace: addon.Namespace,
		},
		Spec: promv1.PrometheusSpec{
			//RuleSelector: prometheusSelector,
			CommonPrometheusFields: promv1.CommonPrometheusFields{
				ExternalLabels: map[string]string{
					"cluster_id": clusterID,
				},
				LogLevel: "debug",
				RemoteWrite: []promv1.RemoteWriteSpec{
					{
						WriteRelabelConfigs: []promv1.RelabelConfig{
							{
								Action: "keep",
								Regex:  regex, //"csv_succeeded$|csv_abnormal$|cluster_version$|ALERTS$|subscription_sync_total|trino_.*$|jvm_heap_memory_used$|node_.*$|namespace_.*$|kube_.*$|cluster.*$|container_.*$",
							},
						},
						URL: remoteWriteURL,
						TLSConfig: &promv1.TLSConfig{
							SafeTLSConfig: promv1.SafeTLSConfig{
								InsecureSkipVerify: true,
							},
						},
						OAuth2: &promv1.OAuth2{
							ClientID: promv1.SecretOrConfigMap{
								Secret: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: vaultSecretName, //"starburst-addon",
									},
									Key: "client-id",
								},
							},
							ClientSecret: corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: vaultSecretName, //"starburst-addon",
								},
								Key: "client-secret",
							},
							TokenURL: tokenURL,
						},
					},
				},
				ServiceMonitorNamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": addon.Namespace,
					},
				},

				ServiceMonitorSelector: &metav1.LabelSelector{},
				PodMonitorSelector:     &metav1.LabelSelector{},
				ServiceAccountName:     prometheusName, //"starburst-enterprise-helm-operator-controller-manager",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("400Mi"),
					},
				},
			},
		},
	}
}

// build the desired enterprise name
func buildEnterpriseName(name string) string {
	return fmt.Sprintf("%s-enterprise", name)
}

func fetchClusterID() string {
	return "1v529ivvikohbpg8pgfihegcdjhudjng"
	// clusterID := cv.Spec.ClusterID
	// return string(clusterID)
}
