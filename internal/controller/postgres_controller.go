package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	postgresv1alpha1 "github.com/rezacloner1372/postgresql-operator/api/v1alpha1"
)

// PostgresReconciler reconciles a Postgres object
type PostgresReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=postgres.snappcloud.io,resources=postgreses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgres.snappcloud.io,resources=postgreses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=postgres.snappcloud.io,resources=postgreses/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

func (r *PostgresReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Postgres instance
	var postgres postgresv1alpha1.Postgres
	if err := r.Get(ctx, req.NamespacedName, &postgres); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Postgres resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Postgres")
		return ctrl.Result{}, err
	}

	// Check if the resource is being deleted
	if !postgres.ObjectMeta.DeletionTimestamp.IsZero() {
		if containsString(postgres.ObjectMeta.Finalizers, "postgres.finalizer") {
			if err := r.finalizerPostgres(&postgres); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer after cleanup
			postgres.ObjectMeta.Finalizers = removeString(postgres.ObjectMeta.Finalizers, "postgres.finalizer")
			if err := r.Update(ctx, &postgres); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil

	}

	// Add finalizer if not exist
	if !containsString(postgres.ObjectMeta.Finalizers, "postgres.finalizer") {
		postgres.ObjectMeta.Finalizers = append(postgres.ObjectMeta.Finalizers, "postgres.finalizer")
		if err := r.Update(ctx, &postgres); err != nil {
			return ctrl.Result{}, nil
		}
	}

	// Fetch the refrenced secret for db credentials
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: postgres.Spec.Auth.SecretRef, Namespace: req.Namespace}, &secret); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Referenced Secret not found", "Secret", postgres.Spec.Auth.SecretRef)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Failed to get Secret", "Secret", postgres.Spec.Auth.SecretRef)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Ensure the statefulset is existing
	statefulsetName := postgres.Name
	var statefulset appsv1.StatefulSet
	err := r.Get(ctx, types.NamespacedName{Name: statefulsetName, Namespace: req.Namespace}, &statefulset)
	if err != nil {
		if errors.IsNotFound(err) {
			// Define a new StatefulSet
			sts := r.statefulSetForPostgres(&postgres, &secret)

			// Set the Postgres instance as the owner and controller of the StatefulSet
			if err := ctrl.SetControllerReference(&postgres, sts, r.Scheme); err != nil {
				logger.Error(err, "Failed to set owner reference on StatefulSet")
				return ctrl.Result{}, err
			}

			logger.Info("Creating a new StatefulSet", "StatefulSet.Namespace", sts.Namespace, "StatefulSet.Name", sts.Name)
			if err := r.Create(ctx, sts); err != nil {
				logger.Error(err, "Failed to create new StatefulSet", "StatefulSet.Namespace", sts.Namespace, "StatefulSet.Name", sts.Name)
				return ctrl.Result{}, err
			}
			// StatefulSet created successfully - return and requeue
			return ctrl.Result{Requeue: true}, nil
		} else {
			logger.Error(err, "Failed to get StatefulSet")
			return ctrl.Result{}, err
		}
	}

	// Ensure the service is existing
	serviceName := "postgres-service"
	var service corev1.Service
	err = r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: req.Namespace}, &service)
	if err != nil && errors.IsNotFound(err) {
		// Define a new Service
		svc := r.serviceForPostgres(&postgres)

		// Set the Postgres instance as the owner and controller of the StatefulSet
		if err := ctrl.SetControllerReference(&postgres, svc, r.Scheme); err != nil {
			logger.Error(err, "Failed to set owner reference on Service")
			return ctrl.Result{}, err
		}
		logger.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		if err := r.Create(ctx, svc); err != nil {
			logger.Error(err, "Failed to create new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			return ctrl.Result{}, err
		}
		// Service created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// Check if the StatefulSet is ready
	if statefulset.Status.ReadyReplicas != *statefulset.Spec.Replicas {
		if postgres.Status.Ready {
			postgres.Status.Ready = false

			if err := r.Status().Update(ctx, &postgres); err != nil {
				logger.Error(err, "unable to update Postgres status")
				return ctrl.Result{}, err
			}
		}
		logger.Info("StatefulSet is not ready yet", "StatefulSet.Name", statefulset.Name)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil // allows the operator to requeue after 5.
	}

	// If StatefulSet is ready, ensure Postgres Ready status is true
	if !postgres.Status.Ready {
		postgres.Status.Ready = true
		if err := r.Status().Update(ctx, &postgres); err != nil {
			logger.Error(err, "unable to update Postgres status")
			return ctrl.Result{}, err
		}
		logger.Info("Postgres resource is ready", "Postgres.Name", postgres.Name)

	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgresReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&postgresv1alpha1.Postgres{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// Helper function statefulSetForPostgres returns a StatefulSet object that will be created
func (r *PostgresReconciler) statefulSetForPostgres(pg *postgresv1alpha1.Postgres, secret *corev1.Secret) *appsv1.StatefulSet {
	labels := map[string]string{
		"app": pg.Name,
	}
	replicas := int32(1)

	return &appsv1.StatefulSet{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      pg.Name,
			Namespace: pg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "postgres-service",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "postgresql",
						Image: "postgres:" + pg.Spec.Version,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 5432,
							Name:          "postgres",
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "data",
							MountPath: "/var/lib/postgresql/data",
						}},
						Env: []corev1.EnvVar{
							{
								Name:  "POSTGRES_DB",
								Value: pg.Spec.Auth.Database,
							},
							{
								Name: "POSTGRES_USER",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: pg.Spec.Auth.SecretRef,
										},
										Key: "username",
									},
								},
							},
							{
								Name: "POSTGRES_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: pg.Spec.Auth.SecretRef,
										},
										Key: "password",
									},
								},
							},
						},
						Lifecycle: &corev1.Lifecycle{
							PostStart: &corev1.LifecycleHandler{
								Exec: &corev1.ExecAction{
									Command: []string{
										"/bin/sh",
										"-c",
										"sed -i 's/trust/md5/g' /var/lib/postgresql/data/pg_hba.conf && echo 'host all all all md5' >> /var/lib/postgresql/data/pg_hba.conf",
									},
								},
							},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse(pg.Spec.Persistence.Size),
						},
					},
				},
			}},
		},
	}
}

// Helper function serviceForPostgres returns a Service object to expose the Postgres
func (r *PostgresReconciler) serviceForPostgres(pg *postgresv1alpha1.Postgres) *corev1.Service {
	labels := map[string]string{
		"app": pg.Name,
	}

	return &corev1.Service{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      "postgres-service",
			Namespace: pg.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Port:     5432,
				Name:     "postgres",
				Protocol: corev1.ProtocolTCP,
			}},
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

func (r *PostgresReconciler) finalizerPostgres(postgres *postgresv1alpha1.Postgres) error {
	statefulsetName := postgres.Name
	var statefulset appsv1.StatefulSet
	err := r.Get(context.TODO(), types.NamespacedName{Name: statefulsetName, Namespace: postgres.Namespace}, &statefulset)
	if err == nil {
		if err := r.Delete(context.TODO(), &statefulset); err != nil {
			return err
		}
	}
	log.Log.Info("Finalizer cleanup started for Postgres", "Postgres.Name", postgres.Name)
	time.Sleep(10 * time.Second)

	serviceName := "postgres-service"
	var service corev1.Service
	err = r.Get(context.TODO(), types.NamespacedName{Name: serviceName, Namespace: postgres.Namespace}, &service)
	if err == nil {
		if err := r.Delete(context.TODO(), &service); err != nil {
			return err
		}
	}
	return nil
}
