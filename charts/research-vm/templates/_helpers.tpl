{{- define "rvm.name" -}}{{ .Release.Name | trunc 53 | trimSuffix "-" }}{{- end -}}

{{- define "rvm.labels" -}}
app.kubernetes.io/name: research-vm
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
kubetre.io/workspace: {{ .Values.kubetre.workspace | quote }}
kubetre.io/service: {{ .Values.kubetre.service | quote }}
{{- end -}}


{{- define "rvm.reviewCloudInit" -}}
#cloud-config
write_files:
  - path: /home/{{ .Values.username }}/airlock/README.txt
    permissions: "0600"
    owner: {{ .Values.username }}:{{ .Values.username }}
    defer: true
    content: |
      This VM reviews an airlock request. The request's file can be downloaded, read-only,
      with the link in link.txt, valid for 8 hours from when the VM was requested:

        curl -o request-file "$(sed 's#?#/<file name>?#' link.txt)"

      or, with azcopy:

        azcopy copy "$(cat link.txt)" . --recursive
  - path: /home/{{ .Values.username }}/airlock/link.txt
    permissions: "0600"
    owner: {{ .Values.username }}:{{ .Values.username }}
    defer: true
    content: {{ .Values.airlockRequestUrl | quote }}
{{- end -}}
