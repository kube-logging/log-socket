package controllers

// import (
// 	"context"

// 	"github.com/go-logr/logr"
// 	ctrl "sigs.k8s.io/controller-runtime"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// )

// func NewLoggingReconciler(client client.Client, log logr.Logger) *LoggingReconciler {
// 	return &LoggingReconciler{
// 		Client: client,
// 		Log:    log,
// 	}
// }

// type LoggingReconciler struct {
// 	client.Client
// 	Log logr.Logger
// }

// func (r *LoggingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

// 	return nil, nil
// }

// func SetupLoggingWithManager(mgr ctrl.Manager, logger logr.Logger) *ctrl.Builder {
// 	return nil
// }
