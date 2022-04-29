package reconciler

import (
	"context"
	"errors"
	"path"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/log-socket/internal"
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func New(ingestAddr string, client client.Client) *Reconciler {
	return &Reconciler{IngestAddr: ingestAddr, Client: client}
}

type Reconciler struct {
	Client     client.Client
	IngestAddr string
}

type UpdateReference func(refs []string) []string

type OutputReference string

func (o OutputReference) Add(refs []string) []string {
	for _, v := range refs {
		if v == string(o) {
			return refs
		}
	}
	return append(refs, string(o))
}

func (o OutputReference) Remove(refs []string) []string {
	for i := range refs {
		if refs[i] == string(o) {
			return append(refs[:i], refs[i+1:]...)
		}
	}
	return refs
}

func (r *Reconciler) Reconcile(ctx context.Context, event internal.ReconcileEvent) (res ctrl.Result, err error) {

	var OutputList loggingv1beta1.OutputList
	var ClusterOutputList loggingv1beta1.ClusterOutputList

	if err := r.Client.List(ctx, &OutputList, client.MatchingLabels(internal.DefLabel)); err != nil {
		return res, err
	}
	if err := r.Client.List(ctx, &ClusterOutputList, client.MatchingLabels(internal.DefLabel)); err != nil {
		return res, err
	}

	outputMap := map[types.NamespacedName]client.Object{}
	for _, output := range OutputList.Items {
		outputMap[client.ObjectKeyFromObject(&output)] = &output
	}
	for _, clusterOutput := range ClusterOutputList.Items {
		outputMap[client.ObjectKeyFromObject(&clusterOutput)] = &clusterOutput
	}

	result := reconciler.CombinedResult{}
	for _, req := range event.Requests {
		outputName := types.NamespacedName{Namespace: req.Namespace, Name: generateOutputName(req.Name)}
		if _, ok := outputMap[outputName]; !ok {
			res, err := r.EnsureOutput(ctx, req)
			result.Combine(&res, err)
		}
		delete(outputMap, outputName)
	}

	// handle removed outputs
	for _, v := range outputMap {
		r.RemoveOutput(ctx, v)
	}

	return result.Result, result.Err
}

func (r *Reconciler) RemoveOutput(ctx context.Context, obj client.Object) (res ctrl.Result, err error) {
	flowName := obj.GetAnnotations()[internal.FlowAnnotationKey]
	res, err = r.ReconcileFlow(ctx, internal.FlowReference{
		NamespacedName: types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      flowName},
		Kind: getFlowKind(obj)},
		OutputReference(obj.GetName()).Remove)

	if client.IgnoreNotFound(err) != nil {
		return
	}

	err = r.Client.Delete(ctx, obj)

	return
}

func (r *Reconciler) EnsureOutput(ctx context.Context, flowRef internal.FlowReference) (res ctrl.Result, err error) {
	var obj client.Object
	meta := r.OutputObjectMeta(types.NamespacedName{Namespace: flowRef.Namespace, Name: generateOutputName(flowRef.Name)}, flowRef.Name)
	spec := loggingv1beta1.OutputSpec{
		HTTPOutput: r.HTTPOuput(flowRef),
	}
	switch flowRef.Kind {
	case internal.FKClusterFlow:
		obj = &loggingv1beta1.ClusterOutput{
			ObjectMeta: meta,
			Spec: loggingv1beta1.ClusterOutputSpec{
				OutputSpec: spec,
			},
		}
	default:
		obj = &loggingv1beta1.Output{
			ObjectMeta: meta,
			Spec:       spec,
		}
	}

	err = r.Client.Create(ctx, obj)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return
	}

	res, err = r.ReconcileFlow(ctx, flowRef, OutputReference(obj.GetName()).Add)

	return
}

func (r *Reconciler) OutputObjectMeta(key types.NamespacedName, flowName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace:   key.Namespace,
		Name:        key.Name,
		Labels:      internal.DefLabel,
		Annotations: map[string]string{internal.FlowAnnotationKey: flowName},
	}
}

func (r *Reconciler) HTTPOuput(flowRef internal.FlowReference) *output.HTTPOutputConfig {
	path.Join()
	return &output.HTTPOutputConfig{
		Endpoint: strings.TrimRight(r.IngestAddr, "/") + "/" + flowRef.URL(),
		Format: &output.Format{
			Type: "json",
		},
		Buffer: &output.Buffer{
			Type:      "memory",
			FlushMode: "immediate",
		},
	}
}

func (r *Reconciler) ReconcileFlow(ctx context.Context, ref internal.FlowReference, updater UpdateReference) (res ctrl.Result, err error) {
	if updater == nil {
		return res, errors.New("no update function added")
	}
	switch ref.Kind {
	case internal.FKClusterFlow:
		var clusterFlow loggingv1beta1.ClusterFlow
		if err = r.Client.Get(ctx, ref.NamespacedName, &clusterFlow); err != nil {
			return
		}
		clusterFlow.Spec.GlobalOutputRefs = updater(clusterFlow.Spec.GlobalOutputRefs)
		if err = r.Client.Update(ctx, &clusterFlow); err != nil {
			return
		}
		return
	default:
		var flow loggingv1beta1.Flow
		if err = r.Client.Get(ctx, ref.NamespacedName, &flow); err != nil {
			return
		}
		flow.Spec.GlobalOutputRefs = updater(flow.Spec.GlobalOutputRefs)
		if err = r.Client.Update(ctx, &flow); err != nil {
			return
		}
		return
	}
}

func generateOutputName(name string) string {
	return name + "-tailer"
}

func getFlowKind(v client.Object) internal.FlowKind {
	kind := internal.FKFlow
	_, ok := v.(*loggingv1beta1.ClusterOutput)
	if ok {
		kind = internal.FKClusterFlow
	}
	return kind
}
