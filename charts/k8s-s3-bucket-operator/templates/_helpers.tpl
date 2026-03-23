{{/*
Expand the name of the chart.
*/}}
{{- define "k8s-s3-bucket-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "k8s-s3-bucket-operator.fullname" -}}
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

{{/*
Chart label
*/}}
{{- define "k8s-s3-bucket-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "k8s-s3-bucket-operator.labels" -}}
helm.sh/chart: {{ include "k8s-s3-bucket-operator.chart" . }}
{{ include "k8s-s3-bucket-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "k8s-s3-bucket-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-s3-bucket-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: {{ include "k8s-s3-bucket-operator.name" . }}
{{- end }}

{{/*
Service account name
*/}}
{{- define "k8s-s3-bucket-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "k8s-s3-bucket-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Operator image reference
*/}}
{{- define "k8s-s3-bucket-operator.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{/*
Namespace for all namespaced resources
*/}}
{{- define "k8s-s3-bucket-operator.namespace" -}}
{{- .Values.namespace.name }}
{{- end }}

{{/*
Secret name for MinIO env
*/}}
{{- define "k8s-s3-bucket-operator.minioSecretName" -}}
{{- if .Values.minio.existingSecret }}
{{- .Values.minio.existingSecret }}
{{- else }}
{{- .Values.minio.secretName }}
{{- end }}
{{- end }}

{{/*
ClusterRole name (cluster-wide; default from release fullname)
*/}}
{{- define "k8s-s3-bucket-operator.clusterRoleName" -}}
{{- default (printf "%s-role" (include "k8s-s3-bucket-operator.fullname" .)) .Values.rbac.clusterRoleName }}
{{- end }}

{{/*
ClusterRoleBinding name
*/}}
{{- define "k8s-s3-bucket-operator.clusterRoleBindingName" -}}
{{- default (printf "%s-rolebinding" (include "k8s-s3-bucket-operator.fullname" .)) .Values.rbac.clusterRoleBindingName }}
{{- end }}
