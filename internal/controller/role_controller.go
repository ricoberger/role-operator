package controller

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ricobergerdev1alpha1 "github.com/ricoberger/role-operator/api/v1alpha1"
)

// RoleReconciler reconciles a Role object
type RoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ricoberger.de,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ricoberger.de,resources=roles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ricoberger.de,resources=roles/finalizers,verbs=update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete;bind;escalate
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *RoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	role := &ricobergerdev1alpha1.Role{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		// The Role was deleted, owner references take care of garbage
		// collecting the managed objects.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// A Role without subjects grants nothing, therefore we treat an empty list
	// of subjects as a spec error. We do not create any objects and prune the
	// ones which may already exist. We do not requeue, because a requeue can
	// not fix a spec problem.
	if len(role.Spec.Subjects) == 0 {
		if err := r.pruneAll(ctx, role); err != nil {
			log.Error(err, "failed to reconcile Role")
			if updateReadyConditionErr := r.updateReadyCondition(ctx, role, metav1.ConditionFalse, "ReconcileFailed", err.Error()); updateReadyConditionErr != nil {
				log.Error(updateReadyConditionErr, "failed to update Ready condition")
				return ctrl.Result{}, utilerrors.NewAggregate([]error{updateReadyConditionErr, err})
			}
			return ctrl.Result{}, err
		}
		log.Info("Role has no subjects, skipping reconciliation")
		if updateReadyConditionErr := r.updateReadyCondition(ctx, role, metav1.ConditionFalse, "NoSubjects", "spec.subjects must contain at least one subject"); updateReadyConditionErr != nil {
			log.Error(updateReadyConditionErr, "failed to update Ready condition")
			return ctrl.Result{}, updateReadyConditionErr
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcile(ctx, role); err != nil {
		log.Error(err, "failed to reconcile Role")
		if updateReadyConditionErr := r.updateReadyCondition(ctx, role, metav1.ConditionFalse, "ReconcileFailed", err.Error()); updateReadyConditionErr != nil {
			log.Error(updateReadyConditionErr, "failed to update Ready condition")
			return ctrl.Result{}, utilerrors.NewAggregate([]error{updateReadyConditionErr, err})
		}
		return ctrl.Result{}, err
	}

	log.Info("Role reconciled successfully")
	if err := r.updateReadyCondition(ctx, role, metav1.ConditionTrue, "ReconcileSucceeded", "Role reconciled successfully"); err != nil {
		log.Error(err, "failed to update Ready condition")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcile applies the desired state for all managed objects and prunes the
// ones which are no longer desired.
func (r *RoleReconciler) reconcile(ctx context.Context, role *ricobergerdev1alpha1.Role) error {
	var errs []error

	if err := r.applyRoles(ctx, role); err != nil {
		errs = append(errs, err)
	}
	if err := r.applyRoleBindings(ctx, role); err != nil {
		errs = append(errs, err)
	}
	if err := r.applyClusterRole(ctx, role); err != nil {
		errs = append(errs, err)
	}
	if err := r.applyClusterRoleBinding(ctx, role); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

// desiredNamespaces returns the set of namespaces from the Role spec which
// currently exist in the cluster. Namespaces which do not exist are skipped
// silently, mirroring the behavior of the Helm chart's `lookup`. When no
// roleRules are set, no namespaces are desired.
func (r *RoleReconciler) desiredNamespaces(ctx context.Context, role *ricobergerdev1alpha1.Role) (map[string]bool, error) {
	namespaces := map[string]bool{}

	if len(role.Spec.RoleRules) == 0 {
		return namespaces, nil
	}

	for _, name := range role.Spec.Namespaces {
		ns := &corev1.Namespace{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		namespaces[name] = true
	}

	return namespaces, nil
}

// applyRoles creates or updates the Roles in all desired namespaces and prunes
// Roles in namespaces which are no longer desired.
func (r *RoleReconciler) applyRoles(ctx context.Context, role *ricobergerdev1alpha1.Role) error {
	namespaces, err := r.desiredNamespaces(ctx, role)
	if err != nil {
		return err
	}

	var errs []error
	for namespace := range namespaces {
		obj := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      role.Name,
				Namespace: namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
			setLabels(obj, role)
			obj.Rules = role.Spec.RoleRules
			return controllerutil.SetControllerReference(role, obj, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to reconcile Role %s/%s: %w", namespace, role.Name, err))
		}
	}

	if err := r.pruneRoles(ctx, role, namespaces); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

// applyRoleBindings creates or updates the RoleBindings in all desired
// namespaces and prunes RoleBindings in namespaces which are no longer desired.
func (r *RoleReconciler) applyRoleBindings(ctx context.Context, role *ricobergerdev1alpha1.Role) error {
	namespaces, err := r.desiredNamespaces(ctx, role)
	if err != nil {
		return err
	}

	var errs []error
	for namespace := range namespaces {
		obj := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      role.Name,
				Namespace: namespace,
			},
		}
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
			setLabels(obj, role)
			obj.Subjects = role.Spec.Subjects
			obj.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     role.Name,
			}
			return controllerutil.SetControllerReference(role, obj, r.Scheme)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to reconcile RoleBinding %s/%s: %w", namespace, role.Name, err))
		}
	}

	if err := r.pruneRoleBindings(ctx, role, namespaces); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

// applyClusterRole creates or updates the ClusterRole when clusterRoleRules are
// set, otherwise it prunes an existing ClusterRole.
func (r *RoleReconciler) applyClusterRole(ctx context.Context, role *ricobergerdev1alpha1.Role) error {
	if len(role.Spec.ClusterRoleRules) == 0 {
		return r.deleteIfExists(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: role.Name}})
	}

	obj := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: role.Name}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		setLabels(obj, role)
		obj.Rules = role.Spec.ClusterRoleRules
		return controllerutil.SetControllerReference(role, obj, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile ClusterRole %s: %w", role.Name, err)
	}
	return nil
}

// applyClusterRoleBinding creates or updates the ClusterRoleBinding when
// clusterRoleRules are set, otherwise it prunes an existing
// ClusterRoleBinding.
func (r *RoleReconciler) applyClusterRoleBinding(ctx context.Context, role *ricobergerdev1alpha1.Role) error {
	if len(role.Spec.ClusterRoleRules) == 0 {
		return r.deleteIfExists(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: role.Name}})
	}

	obj := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: role.Name}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		setLabels(obj, role)
		obj.Subjects = role.Spec.Subjects
		obj.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     role.Name,
		}
		return controllerutil.SetControllerReference(role, obj, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile ClusterRoleBinding %s: %w", role.Name, err)
	}
	return nil
}

// pruneAll removes every object managed for the given Role. It is used when the
// Role has no subjects and therefore should not manage any RBAC objects.
func (r *RoleReconciler) pruneAll(ctx context.Context, role *ricobergerdev1alpha1.Role) error {
	var errs []error
	if err := r.pruneRoles(ctx, role, nil); err != nil {
		errs = append(errs, err)
	}
	if err := r.pruneRoleBindings(ctx, role, nil); err != nil {
		errs = append(errs, err)
	}
	if err := r.deleteIfExists(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: role.Name}}); err != nil {
		errs = append(errs, err)
	}
	if err := r.deleteIfExists(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: role.Name}}); err != nil {
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

// pruneRoles deletes managed Roles which are not in the set of desired
// namespaces.
func (r *RoleReconciler) pruneRoles(ctx context.Context, role *ricobergerdev1alpha1.Role, keep map[string]bool) error {
	list := &rbacv1.RoleList{}
	if err := r.List(ctx, list, client.MatchingLabels(ownerLabels(role))); err != nil {
		return err
	}

	var errs []error
	for i := range list.Items {
		item := &list.Items[i]
		if keep[item.Namespace] {
			continue
		}
		if err := r.Delete(ctx, item); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// pruneRoleBindings deletes managed RoleBindings which are not in the set of
// desired namespaces.
func (r *RoleReconciler) pruneRoleBindings(ctx context.Context, role *ricobergerdev1alpha1.Role, keep map[string]bool) error {
	list := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, list, client.MatchingLabels(ownerLabels(role))); err != nil {
		return err
	}

	var errs []error
	for i := range list.Items {
		item := &list.Items[i]
		if keep[item.Namespace] {
			continue
		}
		if err := r.Delete(ctx, item); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// deleteIfExists deletes the given object, ignoring not found errors.
func (r *RoleReconciler) deleteIfExists(ctx context.Context, obj client.Object) error {
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// updateReadyCondition sets the Ready condition and persists the status. It only
// writes when the condition actually changed and retries on conflict against a
// freshly read object, so that concurrent reconciles (e.g. triggered by the
// owned RBAC objects) do not produce spurious "object has been modified"
// errors.
func (r *RoleReconciler) updateReadyCondition(ctx context.Context, role *ricobergerdev1alpha1.Role, status metav1.ConditionStatus, reason, message string) error {
	key := client.ObjectKeyFromObject(role)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &ricobergerdev1alpha1.Role{}
		if err := r.Get(ctx, key, current); err != nil {
			return err
		}
		if !meta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: current.Generation,
		}) {
			return nil
		}
		return r.Status().Update(ctx, current)
	})
}

// setLabels sets the operator's identifying labels on the given object.
func setLabels(obj client.Object, role *ricobergerdev1alpha1.Role) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["app.kubernetes.io/managed-by"] = "role-operator"
	labels["role-operator.ricoberger.de/role"] = role.Name
	obj.SetLabels(labels)
}

// ownerLabels returns the label selector which matches all objects managed for
// the given Role.
func ownerLabels(role *ricobergerdev1alpha1.Role) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by":     "role-operator",
		"role-operator.ricoberger.de/role": role.Name,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ricobergerdev1alpha1.Role{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.mapNamespaceToRoles),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc:  func(event.CreateEvent) bool { return true },
				UpdateFunc:  func(event.UpdateEvent) bool { return false },
				DeleteFunc:  func(event.DeleteEvent) bool { return false },
				GenericFunc: func(event.GenericEvent) bool { return false },
			}),
		).
		Named("role").
		Complete(r)
}

// mapNamespaceToRoles enqueues all Role CRs which reference the created
// namespace, so that their Roles/RoleBindings are created promptly.
func (r *RoleReconciler) mapNamespaceToRoles(ctx context.Context, obj client.Object) []reconcile.Request {
	namespace := obj.GetName()

	roles := &ricobergerdev1alpha1.RoleList{}
	if err := r.List(ctx, roles); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for i := range roles.Items {
		role := &roles.Items[i]
		if slices.Contains(role.Spec.Namespaces, namespace) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: role.Name},
			})
		}
	}
	return requests
}
