#!/usr/bin/env bash
curl localhost:10000/healthz
curl localhost:10000/clusterflow/default/nullout -d '{"foo":{"bar":"baz"}, "kubernetes":{"labels":{"rbac/AllowList":"system_serviceaccount_default_default"}}}'
curl localhost:10000/clusterflow/default/nullout -d '{"foo":{"bar":"baz"}}'
