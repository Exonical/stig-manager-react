{{/*
Expand the name of the chart.
*/}}
{{- define "stig-manager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully-qualified app name. Truncated at 63 chars to fit Kubernetes
DNS-1123 label limits. The chart-level fullnameOverride wins if set;
otherwise we combine the release name + chart name.
*/}}
{{- define "stig-manager.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "stig-manager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels — applied to every resource the chart creates.
*/}}
{{- define "stig-manager.labels" -}}
helm.sh/chart: {{ include "stig-manager.chart" . }}
{{ include "stig-manager.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/*
Selector labels — stable across releases (no version label).
*/}}
{{- define "stig-manager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "stig-manager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Per-component label helpers.
*/}}
{{- define "stig-manager.api.selectorLabels" -}}
{{ include "stig-manager.selectorLabels" . }}
app.kubernetes.io/component: api
{{- end -}}

{{- define "stig-manager.api.labels" -}}
{{ include "stig-manager.labels" . }}
app.kubernetes.io/component: api
{{- end -}}

{{- define "stig-manager.web.selectorLabels" -}}
{{ include "stig-manager.selectorLabels" . }}
app.kubernetes.io/component: web
{{- end -}}

{{- define "stig-manager.web.labels" -}}
{{ include "stig-manager.labels" . }}
app.kubernetes.io/component: web
{{- end -}}

{{/*
Per-component resource names.
*/}}
{{- define "stig-manager.api.fullname" -}}
{{ printf "%s-api" (include "stig-manager.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "stig-manager.web.fullname" -}}
{{ printf "%s-web" (include "stig-manager.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "stig-manager.config.fullname" -}}
{{ printf "%s-config" (include "stig-manager.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "stig-manager.secret.fullname" -}}
{{ printf "%s-secrets" (include "stig-manager.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{- define "stig-manager.web.config.fullname" -}}
{{ printf "%s-web-config" (include "stig-manager.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
ServiceAccount name helpers honour `.create: false` and explicit `.name`.
*/}}
{{- define "stig-manager.api.serviceAccountName" -}}
{{- if .Values.api.serviceAccount.create -}}
{{- default (printf "%s-api" (include "stig-manager.fullname" .)) .Values.api.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.api.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "stig-manager.web.serviceAccountName" -}}
{{- if .Values.web.serviceAccount.create -}}
{{- default (printf "%s-web" (include "stig-manager.fullname" .)) .Values.web.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.web.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
The image tag falls back to .Chart.AppVersion when not explicitly set.
*/}}
{{- define "stig-manager.api.image" -}}
{{- $tag := default .Chart.AppVersion .Values.api.image.tag -}}
{{- printf "%s:%s" .Values.api.image.repository $tag -}}
{{- end -}}

{{- define "stig-manager.web.image" -}}
{{- $tag := default .Chart.AppVersion .Values.web.image.tag -}}
{{- printf "%s:%s" .Values.web.image.repository $tag -}}
{{- end -}}

{{/*
Name of the Secret the API mounts. Returns the existing secret name
when configured, otherwise the chart-managed Secret.
*/}}
{{- define "stig-manager.databaseSecret.name" -}}
{{- if .Values.secrets.existing.name -}}
{{- .Values.secrets.existing.name -}}
{{- else -}}
{{- include "stig-manager.secret.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "stig-manager.databaseSecret.key" -}}
{{- if .Values.secrets.existing.name -}}
{{- default "databaseUrl" .Values.secrets.existing.databaseUrlKey -}}
{{- else -}}
{{- "databaseUrl" -}}
{{- end -}}
{{- end -}}
