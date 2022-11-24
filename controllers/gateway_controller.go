/*
Copyright 2022.

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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"gitee.com/zongzw/bigip-kubernetes-gateway/pkg"
)

type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Adc object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	zlog := log.FromContext(ctx)

	if !pkg.ActiveSIGs.SyncedAtStart {
		<-time.After(100 * time.Millisecond)
		return ctrl.Result{Requeue: true}, nil
	}
	var obj gatewayv1beta1.Gateway

	zlog.V(1).Info("handling " + req.NamespacedName.String())
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// delete resources
			gw := pkg.ActiveSIGs.GetGateway(req.NamespacedName.String())
			if ocfgs, err := pkg.ParseRelatedForClass(string(gw.Spec.GatewayClassName), []*gatewayv1beta1.Gateway{gw}, nil, nil); err != nil {
				return ctrl.Result{}, err
			} else {
				zlog.V(1).Info("handling + deleting " + req.NamespacedName.String())
				hrs := pkg.ActiveSIGs.AttachedHTTPRoutes(gw)
				pkg.ActiveSIGs.UnsetGateway(req.NamespacedName.String())
				if ncfgs, err := pkg.ParseRelatedForClass(string(gw.Spec.GatewayClassName), nil, hrs, nil); err != nil {
					return ctrl.Result{}, err
				} else {
					pkg.PendingDeploys <- pkg.DeployRequest{
						Meta: fmt.Sprintf("deleting gateway '%s'", req.NamespacedName.String()),
						From: &ocfgs,
						To:   &ncfgs,
						StatusFunc: func() {
							// do something
						},
						Partition: string(gw.Spec.GatewayClassName),
					}
				}
			}

			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, err
		}
	} else {
		// upsert resources
		zlog.V(1).Info("handling + upserting " + req.NamespacedName.String())
		ogw := pkg.ActiveSIGs.GetGateway(req.NamespacedName.String())
		if ogw == nil {
			ogw = &obj
			pkg.ActiveSIGs.SetGateway(obj.DeepCopy())
		}

		ohrs := pkg.ActiveSIGs.AttachedHTTPRoutes(ogw)
		if ocfgs, err := pkg.ParseRelatedForClass(string(ogw.Spec.GatewayClassName), []*gatewayv1beta1.Gateway{ogw}, nil, nil); err != nil {
			zlog.Error(err, "handling + upserting + parse related ocfgs "+req.NamespacedName.String())
			return ctrl.Result{}, err
		} else {
			ngw := obj.DeepCopy()
			if ngw.Spec.GatewayClassName == ogw.Spec.GatewayClassName {
				pkg.ActiveSIGs.SetGateway(ngw)
				if ncfgs, err := pkg.ParseRelatedForClass(string(ngw.Spec.GatewayClassName), []*gatewayv1beta1.Gateway{ngw}, nil, nil); err != nil {
					zlog.Error(err, "handling + upserting + parse related ncfgs "+req.NamespacedName.String())
					return ctrl.Result{}, err
				} else {
					pkg.PendingDeploys <- pkg.DeployRequest{
						Meta: fmt.Sprintf("upserting gateway '%s'", req.NamespacedName.String()),
						From: &ocfgs,
						To:   &ncfgs,
						StatusFunc: func() {
							// do something
						},
						Partition: string(ngw.Spec.GatewayClassName),
					}
					return ctrl.Result{}, nil
				}
			} else {
				// original state of new gatewayclass env
				ocfgsN, err := pkg.ParseRelatedForClass(string(ngw.Spec.GatewayClassName), nil, ohrs, nil)
				if err != nil {
					return ctrl.Result{}, err
				}
				pkg.ActiveSIGs.SetGateway(ngw)

				// gateway is go away
				if ncfgsO, err := pkg.ParseRelatedForClass(string(ogw.Spec.GatewayClassName), nil, ohrs, nil); err != nil {
					return ctrl.Result{}, err
				} else {
					pkg.PendingDeploys <- pkg.DeployRequest{
						Meta: fmt.Sprintf("upserting gateway '%s'", req.NamespacedName.String()),
						From: &ocfgs,
						To:   &ncfgsO,
						StatusFunc: func() {
							// do something
						},
						Partition: string(ogw.Spec.GatewayClassName),
					}
				}

				if ncfgs, err := pkg.ParseRelatedForClass(string(ngw.Spec.GatewayClassName), []*gatewayv1beta1.Gateway{ngw}, nil, nil); err != nil {
					return ctrl.Result{}, err
				} else {
					pkg.PendingDeploys <- pkg.DeployRequest{
						Meta: fmt.Sprintf("upserting gateway '%s'", req.NamespacedName.String()),
						From: &ocfgsN,
						To:   &ncfgs,
						StatusFunc: func() {
							// do something
						},
						Partition: string(ngw.Spec.GatewayClassName),
					}
				}
				return ctrl.Result{}, nil
			}
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1beta1.Gateway{}).
		Complete(r)
}
