{{- define "ohdsi.name" -}}{{ .Release.Name | trunc 40 | trimSuffix "-" }}{{- end -}}

{{- define "ohdsi.labels" -}}
app.kubernetes.io/part-of: ohdsi
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}

{{- define "ohdsi.selector" -}}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/name: {{ .component }}
{{- end -}}

{{- define "ohdsi.podSecurity" -}}
runAsNonRoot: true
runAsUser: {{ .uid }}
runAsGroup: {{ .uid }}
fsGroup: {{ .uid }}
seccompProfile: {type: RuntimeDefault}
{{- end -}}

{{- define "ohdsi.containerSecurity" -}}
allowPrivilegeEscalation: false
capabilities: {drop: ["ALL"]}
{{- end -}}

{{- define "ohdsi.jdbc" -}}
jdbc:postgresql://{{ include "ohdsi.name" . }}-db:5432/ohdsi
{{- end -}}
