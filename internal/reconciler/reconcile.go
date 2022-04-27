package reconciler

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/banzaicloud/log-socket/internal"
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/output"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
)

func New(ingestAddr string, client client.Client) *Reconciler {
	return &Reconciler{IngestAddr: ingestAddr, Client: client}
}

type Reconciler struct {
	Client     client.Client
	IngestAddr string
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

	outputMap := map[types.NamespacedName]interface{}{}
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
			// creating new outputs
			res, err := r.ReconcileOutput(ctx, outputName, reconciler.StatePresent, req.String())
			result.Combine(&res, err)

			// updating flows
			{
				res, err := r.ReconcileFlow(ctx, req, false)
				result.Combine(&res, err)
			}
		}
		delete(outputMap, req)
	}

	// handle removed outputs
	for key := range outputMap {
		// updating flow first
		res, err := r.ReconcileFlow(ctx, key, true)
		result.Combine(&res, err)

		// removing unused outputs
		{
			res, err := r.ReconcileOutput(ctx, key, reconciler.StateAbsent, "")
			result.Combine(&res, err)
		}
	}

	return result.Result, result.Err
}

func (r *Reconciler) ReconcileOutput(ctx context.Context, key client.ObjectKey, state reconciler.DesiredState, flowName string) (res ctrl.Result, err error) {

	var outputBase client.Object

	if key.Namespace == "" {
		outputBase = &loggingv1beta1.ClusterOutput{
			ObjectMeta: metav1.ObjectMeta{
				Name: key.Name,
			},
			Spec: loggingv1beta1.ClusterOutputSpec{
				OutputSpec: loggingv1beta1.OutputSpec{
					HTTPOutput: r.HTTPOuput(map[string]string{internal.FlowNameHeaderKey: flowName}),
				},
			},
		}
	} else {
		outputBase = &loggingv1beta1.Output{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      key.Name,
			},
			Spec: loggingv1beta1.OutputSpec{
				HTTPOutput: r.HTTPOuput(map[string]string{internal.FlowNameHeaderKey: flowName}),
			},
		}
	}

	switch state {
	case reconciler.StatePresent:
		err = r.Client.Create(ctx, outputBase)
		return res, err
	case reconciler.StateAbsent:
		err = r.Client.Delete(ctx, outputBase)
		return res, err
	default:
		return res, errors.New("unknow state")
	}
}

func (r *Reconciler) HTTPOuput(headers map[string]string) *output.HTTPOutputConfig {
	return &output.HTTPOutputConfig{
		Headers:  headers,
		Endpoint: r.IngestAddr,
		Format: &output.Format{
			Type: "json",
		},
	}
}

func (r *Reconciler) ReconcileFlow(ctx context.Context, key client.ObjectKey, removeFlow bool) (res ctrl.Result, err error) {
	outputName := generateOutputName(key.Name)
	switch key.Namespace {
	case "":
		var clusterFlow loggingv1beta1.ClusterFlow
		if err = r.Client.Get(ctx, key, &clusterFlow); err != nil {
			return
		}
		if removeFlow {
			clusterFlow.Spec.GlobalOutputRefs = removeLine(clusterFlow.Spec.GlobalOutputRefs, outputName)
		} else {
			clusterFlow.Spec.GlobalOutputRefs = append(clusterFlow.Spec.GlobalOutputRefs, outputName)
		}
		if err = r.Client.Update(ctx, &clusterFlow); err != nil {
			return
		}
		return
	default:
		var flow loggingv1beta1.Flow
		if err = r.Client.Get(ctx, key, &flow); err != nil {
			return
		}
		if removeFlow {
			flow.Spec.LocalOutputRefs = removeLine(flow.Spec.LocalOutputRefs, outputName)
		} else {
			flow.Spec.LocalOutputRefs = append(flow.Spec.LocalOutputRefs, outputName)
		}
		if err = r.Client.Update(ctx, &flow); err != nil {
			return
		}
		return
	}
}

func removeLine(strs []string, str string) []string {
	for i := range strs {
		if strs[i] == str {
			return append(strs[:i], strs[i+1:]...)
		}
	}
	return strs
}

func generateOutputName(name string) string {
	return name + "-tailer"
}
