# Default values for helm-try.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

replicaCount: 1

image:
  repository: ${image_repo}
  pullPolicy: IfNotPresent
  # Overrides the image tag whose default is the chart appVersion.
  tag: ${version}

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""
env:
- name: KEEPALIVED_OPERATOR_IMAGE_NAME
  value: quay.io/redhat-cop/keepalived-operator:latest
- name: KEEPALIVEDGROUP_TEMPLATE_FILE_NAME
  value: /templates/keepalived-template.yaml
keepalivedTemplateFromConfigMap: "" #i.e. "keepalived-template" of an existing ConfigMap

podAnnotations: {}

resources:
  requests:
    cpu: 100m
    memory: 20Mi

nodeSelector: {}

tolerations: []

affinity: {}

kube_rbac_proxy:
  image:
    repository: quay.io/redhat-cop/kube-rbac-proxy
    pullPolicy: IfNotPresent
    tag: v0.11.0
  resources:
    requests:
      cpu: 100m
      memory: 20Mi

enableMonitoring: true