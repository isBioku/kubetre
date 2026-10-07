{{- define "rvm.name" -}}{{ .Release.Name | trunc 53 | trimSuffix "-" }}{{- end -}}

{{- define "rvm.labels" -}}
app.kubernetes.io/name: research-vm
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/workspace: {{ .Values.kubetre.workspace | quote }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}
