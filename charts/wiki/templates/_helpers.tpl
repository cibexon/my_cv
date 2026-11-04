{{- define "wiki.fullname" -}}
{{- if eq .Release.Name .Chart.Name }}{{ .Release.Name }}{{ else }}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}{{ end }}
{{- end }}

{{- define "wiki.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Values.image.tag | quote }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end }}

{{- define "wiki.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: server
{{- end }}

{{- define "wiki.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag }}
{{- end }}

{{/* Label selector of this release's series in Prometheus (the ServiceMonitor sets namespace and service). */}}
{{- define "wiki.promSelector" -}}
namespace="{{ .Release.Namespace }}",service="{{ include "wiki.fullname" . }}"
{{- end }}

{{/*
Error ratio of one SLO over one window: share of "bad" requests.
  availability: 5xx / all
  latency:      1 - (requests faster than threshold / all)
Called with (dict "root" $ "slo" "availability" "window" "5m").
*/}}
{{- define "wiki.errorRatio" -}}
{{- $sel := include "wiki.promSelector" .root -}}
{{- if eq .slo "availability" -}}
sum(rate(http_requests_total{ {{- $sel }},code=~"5.."}[{{ .window }}]))
/
sum(rate(http_requests_total{ {{- $sel }}}[{{ .window }}]))
{{- else -}}
1 - (
  sum(rate(http_request_duration_seconds_bucket{ {{- $sel }},le="{{ .root.Values.slo.latency.threshold }}"}[{{ .window }}]))
  /
  sum(rate(http_request_duration_seconds_count{ {{- $sel }}}[{{ .window }}]))
)
{{- end -}}
{{- end }}
