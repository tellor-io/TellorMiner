{{/*
Expand the name of the chart.
*/}}
{{- define "telliot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "telliot.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "grafana.fullname" -}}
{{- if .Values.grafana.fullnameOverride }}
{{- .Values.grafana.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "grafana" .Values.grafana.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "prometheus.fullname" -}}
{{- if .Values.prometheus.fullnameOverride }}
{{- .Values.prometheus.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "prometheus" .Values.prometheus.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "alertmanager.fullname" -}}
{{- if .Values.alertmanager.fullnameOverride }}
{{- .Values.alertmanager.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default "alertmanager" .Values.alertmanager.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "telliot.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "telliot.labels" -}}
helm.sh/chart: {{ include "telliot.chart" . }}
{{ include "telliot.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "telliot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "telliot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "telliot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "telliot.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "telliot.modeName" -}}
{{- if eq .mode "dataServer" }}
{{- print "db" }}
{{- else if eq .mode "mine" }}
{{- print "m" }}
{{- else }}
{{- print .mode }}
{{- end }}
{{- end }}