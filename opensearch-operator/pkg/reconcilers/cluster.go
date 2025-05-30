package reconcilers

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	opsterv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/builders"
	"opensearch.opster.io/pkg/helpers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ClusterReconciler struct {
	client.Client
	reconciler.ResourceReconciler
	ctx               context.Context
	recorder          record.EventRecorder
	reconcilerContext *ReconcilerContext
	instance          *opsterv1.OpenSearchCluster
}

func NewClusterReconciler(
	client client.Client,
	ctx context.Context,
	recorder record.EventRecorder,
	reconcilerContext *ReconcilerContext,
	instance *opsterv1.OpenSearchCluster,
	opts ...reconciler.ResourceReconcilerOption,
) *ClusterReconciler {
	return &ClusterReconciler{
		Client: client,
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			append(opts, reconciler.WithLog(log.FromContext(ctx).WithValues("reconciler", "cluster")))...),
		ctx:               ctx,
		recorder:          recorder,
		reconcilerContext: reconcilerContext,
		instance:          instance,
	}
}

func (r *ClusterReconciler) Reconcile() (ctrl.Result, error) {
	//lg := log.FromContext(r.ctx)
	result := reconciler.CombinedResult{}
	username, password, err := helpers.UsernameAndPassword(r.ctx, r.Client, r.instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	clusterService := builders.NewServiceForCR(r.instance)
	result.CombineErr(ctrl.SetControllerReference(r.instance, clusterService, r.Client.Scheme()))
	result.Combine(r.ReconcileResource(clusterService, reconciler.StatePresent))

	discoveryService := builders.NewDiscoveryServiceForCR(r.instance)
	result.CombineErr(ctrl.SetControllerReference(r.instance, discoveryService, r.Scheme()))
	result.Combine(r.ReconcileResource(discoveryService, reconciler.StatePresent))

	passwordSecret := builders.PasswordSecret(r.instance, password)
	result.CombineErr(ctrl.SetControllerReference(r.instance, passwordSecret, r.Scheme()))
	result.Combine(r.ReconcileResource(passwordSecret, reconciler.StatePresent))

	bootstrapPod := builders.NewBootstrapPod(r.instance, r.reconcilerContext.Volumes, r.reconcilerContext.VolumeMounts)
	result.CombineErr(ctrl.SetControllerReference(r.instance, bootstrapPod, r.Scheme()))
	if r.instance.Status.Initialized {
		result.Combine(r.ReconcileResource(bootstrapPod, reconciler.StateAbsent))
	} else {
		result.Combine(r.ReconcileResource(bootstrapPod, reconciler.StatePresent))
	}

	for _, nodePool := range r.instance.Spec.NodePools {
		headlessService := builders.NewHeadlessServiceForNodePool(r.instance, &nodePool)
		result.CombineErr(ctrl.SetControllerReference(r.instance, headlessService, r.Client.Scheme()))
		result.Combine(r.ReconcileResource(headlessService, reconciler.StatePresent))

		result.Combine(r.reconcileNodeStatefulSet(nodePool, username))
	}

	// if Version isn't set we set it now to check for upgrades later.
	if r.instance.Status.Version == "" {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.Get(r.ctx, client.ObjectKeyFromObject(r.instance), r.instance); err != nil {
				return err
			}
			r.instance.Status.Version = r.instance.Spec.General.Version
			return r.Status().Update(r.ctx, r.instance)
		})
		result.CombineErr(err)
	}

	return result.Result, result.Err
}

func (r *ClusterReconciler) reconcileNodeStatefulSet(nodePool opsterv1.NodePool, username string) (*ctrl.Result, error) {
	found, nodePoolConfig := r.reconcilerContext.fetchNodePoolHash(nodePool.Component)
	// If config hasn't been set up for the node pool requeue
	if !found {
		return &ctrl.Result{
			Requeue: true,
		}, nil
	}

	extraConfig := helpers.MergeConfigs(r.instance.Spec.General.AdditionalConfig, nodePool.AdditionalConfig)

	sts := builders.NewSTSForNodePool(
		username,
		r.instance,
		nodePool,
		nodePoolConfig.ConfigHash,
		r.reconcilerContext.Volumes,
		r.reconcilerContext.VolumeMounts,
		extraConfig,
	)
	if err := ctrl.SetControllerReference(r.instance, sts, r.Client.Scheme()); err != nil {
		return &ctrl.Result{}, err
	}

	// First ensure that the statefulset exists
	result, err := r.ReconcileResource(sts, reconciler.StateCreated)
	if err != nil || result != nil {
		return result, err
	}

	// Next get the existing statefulset
	existing := &appsv1.StatefulSet{}
	err = r.Client.Get(r.ctx, client.ObjectKeyFromObject(sts), existing)
	if err != nil {
		return result, err
	}

	// Now set the desired replicas to be the existing replicas
	// This will allow the scaler reconciler to function correctly
	sts.Spec.Replicas = existing.Spec.Replicas

	// Don't update env vars on non data nodes while an upgrade is in progress
	// as we don't want uncontrolled restarts while we're doing an upgrade
	if r.instance.Status.Version != "" &&
		r.instance.Status.Version != r.instance.Spec.General.Version &&
		!helpers.ContainsString(nodePool.Roles, "data") {
		sts.Spec.Template.Spec.Containers[0].Env = existing.Spec.Template.Spec.Containers[0].Env
	}

	// Finally we enforce the desired state
	return r.ReconcileResource(sts, reconciler.StatePresent)
}

func (r *ClusterReconciler) DeleteResources() (ctrl.Result, error) {
	result := reconciler.CombinedResult{}
	return result.Result, result.Err
}
