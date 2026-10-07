{{- define "desktop.name" -}}{{ .Release.Name | trunc 53 | trimSuffix "-" }}{{- end -}}

{{- define "desktop.labels" -}}
app.kubernetes.io/name: linux-desktop
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/workspace: {{ .Values.kubetre.workspace | quote }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}

{{- define "desktop.selector" -}}
app.kubernetes.io/name: linux-desktop
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
