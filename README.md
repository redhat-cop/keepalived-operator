# Keepalived operator

The objective of the keepalived operator provides is to allow for a way to create self-hosted load balancers in an automated way. From a user experience point of view the behavior is the same as of when creating [`LoadBalancer`](https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer)  services with a cloud provider able to manage them.

The keepalived operator can be used in all environments that allows nodes to advertise additional IPs on their NICs (and at least for now, in networks that allow multicast), however it's mainly aimed at supporting LoadBalancer services and ExternalIPs on bare metal installations (or other installation environments where a cloud provider is not available).

One possible use of the keepalived operator is also to support [OpenShift Ingresses](https://docs.openshift.com/container-platform/4.3/networking/configuring-ingress-cluster-traffic/overview-traffic.html) in environments where an external load balancer cannot be provisioned.

## how it works

The keepalived operator will create one or more [VIPs](https://en.wikipedia.org/wiki/Virtual_IP_address) (an HA IP that floats between multiple nodes), based on the [`LoadBalancer`](https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer) services and/or services requesting [`ExternalIPs`](https://kubernetes.io/docs/concepts/services-networking/service/#external-ips).

For `LoadBalancer` services the IPs found at `.Status.LoadBalancer.Ingress[].IP` will become VIPs.

For services requesting a `ExternalIPs`, the IPs found at `.Spec.ExternalIPs[]` will become VIPs.

Note that a service can be of `LoadBalancer` type and also request `ExternalIPs`, it this case both sets of IPs will become VIPs.

Due to a [keepalived](https://www.keepalived.org/manpage.html) limitation a single keepalived cluster can manage up to 256 VIP configurations. Multiple keepalived clusters can coexists in the same network as long as they use different multicast ports [TODO verify this statement].

To address this limitation the `KeepalivedGroup` [CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) has been introduced. This CRD is supposed to be configured by an administrator and allows you to specify a node selector to pick on which nodes the keepalived pods should be deployed. Here is an example:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: KeepalivedGroup
metadata:
  name: keepalivedgroup-router
spec:
  interface: ens3
  nodeSelector:
    node-role.kubernetes.io/loadbalancer: ""
```

This KeepalivedGroup will be deployed on all the nodes with role `loadbalancer`. One must also specify the network device on which the VIPs will be exposed, it is assumed that all the nodes have the same network device configuration.

Services must be annotated to opt-in to being observed by the keepalived operator and to specify which KeepalivedGroup they refer to. The annotation looks like this:

`keepalived-operator.redhat-cop.io/keepalivedgroup: <keepalivedgroup namespace>/<keepalivedgroup-name>`

## Requirements

Each KeepalivedGroup deploys a [daemonset](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/) that requires the [privileged scc](https://docs.openshift.com/container-platform/4.3/authentication/managing-security-context-constraints.html), this permission must be given to the `default` service account in the namespace where the keepalived group is created by and administrator.

```shell
oc adm policy add-scc-to-user privileged -z default -n keepalived-operator
```

For OpenShift users only, it is necessary to allow for `LoadBalancer` VIPS to be automatically assigned by the systems and for `ExternalIPs` to be selected by the users. This can be done by patching the cluster network. Here is an example of the patch:

```yaml
spec:
  externalIP:
    policy:
      allowedCIDRs:
      - ${ALLOWED_CIDR}
    autoAssignCIDRs:
      - "${AUTOASSIGNED_CIDR}"
```

and here is an example of how to apply the patch:

```shell
export ALLOWED_CIDR="192.168.131.128/26"
export AUTOASSIGNED_CIDR="192.168.131.192/26"
oc patch network cluster -p "$(envsubst < ./network-patch.yaml | yq -j .)" --type=merge
```

## Verbatim Configurations

Keepalived has dozens of [configurations](https://www.keepalived.org/manpage.html). At the early stage of this project it's difficult to tell which one should me modeled in the API. Yet, users of this project may still need to use them. To account for that there is a way to pass verbatim options both at the keepalived group level (which maps to the keepalived config `global_defs` section) and at the service level (which maps to the keepalived config `vrrp_instance` section).

KeepalivedGroup-level verbatim configurations can be passed as in the following example:

```yaml
apiVersion: redhatcop.redhat.io/v1alpha1
kind: KeepalivedGroup
metadata:
  name: keepalivedgroup-router
spec:
  interface: ens3
  nodeSelector:
    node-role.kubernetes.io/loadbalancer: ""
  verbatimConfig:  
    vrrp_iptables: my-keepalived
```

this will map to the following `global_defs`:

```
    global_defs {
        router_id keepalivedgroup-router
        vrrp_iptables my-keepalived
    }
```

Service-level verbatim configurations can be passed as in the following example:

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    keepalived-operator.redhat-cop.io/keepalivedgroup: keepalived-operator/keepalivedgroup-router
    keepalived-operator.redhat-cop.io/verbatimconfig: '{ "track_src_ip": "" }'
```

this will map to the following `vrrp_instance` section

```
    vrrp_instance openshift-ingress/router-default {
        interface ens3
        virtual_router_id 1  
        virtual_ipaddress {
          192.168.131.129
        }
        track_src_ip
    }
```

## Metrics collection

Each keepalived pod exposes a [Prometheus](https://prometheus.io/) metrics port at `9650`. Metrics are collected with [keepalived_exporter](github.com/gen2brain/keepalived_exporter), the vailable metrics are described in the project documentation.

When a keepalived group is created a [`PodMonitor`](https://github.com/coreos/prometheus-operator/blob/master/Documentation/api.md#podmonitor) rule to collect those metrics. All PodMonitor resources created that way have the label: `metrics: keepalived`. It is up to you to make sure your Prometheus instance watches for those `PodMonitor` rules. Here is an example of a fragment of a `Prometheus` CR configured to collect the keepalived pod metrics:

```yaml
  podMonitorSelector:
    matchLabels:
      metrics: keepalived
```

## Deploying the Operator

This is a cluster-level operator that you can deploy in any namespace, `keepalived-operator` is recommended.

You can either deploy it using [`Helm`](https://helm.sh/) or creating the manifests directly.

### Deploying with Helm

Here are the instructions to install the latest release with Helm.

```shell
oc new-project keepalived-operator

helm repo add keepalived-operator https://redhat-cop.github.io/keepalived-operator
helm repo update
export keepalived_operator_chart_version=$(helm search repo keepalived-operator/keepalived-operator | grep keepalived-operator/keepalived-operator | awk '{print $2}')

helm fetch keepalived-operator/keepalived-operator --version ${keepalived_operator_chart_version}
helm template keepalived-operator-${keepalived_operator_chart_version}.tgz --namespace keepalived-operator | oc apply -f - -n keepalived-operator

rm keepalived-operator-${keepalived_operator_chart_version}.tgz
```

### Deploying directly with manifests

Here are the instructions to install the latest release creating the manifest directly in OCP.

```shell
git clone git@github.com:redhat-cop/keepalived-operator.git; cd keepalived-operator
oc apply -f deploy/crds/redhatcop.redhat.io_keepalivedgroups_crd.yaml
oc new-project keepalived-operator
oc -n keepalived-operator apply -f deploy
```

## Local Development

Execute the following steps to develop the functionality locally. It is recommended that development be done using a cluster with `cluster-admin` permissions.

```shell
go mod download
```

optionally:

```shell
go mod vendor
```

Using the [operator-sdk](https://github.com/operator-framework/operator-sdk), run the operator locally:

```shell
oc apply -f deploy/crds/redhatcop.redhat.io_keepalivedgroups_crd.yaml
export REPOSITORY=quay.io/<your_repo>/keepalived-operator
docker login $REPOSITORY
make manager docker-build docker-push-latest
export KEEPALIVED_OPERATOR_IMAGE_NAME=${REPOSITORY}:latest
export KEEPALIVEDGROUP_TEMPLATE_FILE_NAME=./build/templates/keepalived-template.yaml
OPERATOR_NAME='keepalived-operator' operator-sdk --verbose up local --namespace ""
```

## Testing

Add an external ID CIDR to your cluster to manage

```shell
export CIDR="192.168.130.128/28"
oc patch network cluster -p "$(envsubts < ./test/externalIP-patch.yaml | yq -j .)" --type=merge
```

create a project that uses a LoadBalancer Service

```shell
oc new-project test-keepalived-operator
oc new-app django-psql-example -n test-keepalived-operator
oc delete route django-psql-example -n test-keepalived-operator
oc patch service django-psql-example -n test-keepalived-operator -p '{"spec":{"type":"LoadBalancer"}}' --type=strategic
export SERVICE_IP=$(oc get svc django-psql-example -n test-keepalived-operator -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

create a keepalivedgroup

```shell
oc adm policy add-scc-to-user privileged -z default -n test-keepalived-operator
oc apply -f ./test/keepalivedgroup.yaml -n test-keepalived-operator
```

annotate the service to be used by keepalived

```shell
oc annotate svc django-psql-example -n test-keepalived-operator keepalived-operator.redhat-cop.io/keepalivedgroup=test-keepalived-operator/keepalivedgroup-test
```

curl the app using the service IP

```shell
curl http://$SERVICE_IP:8080
```

test with a second keepalived group

```shell
oc apply -f ./test/test-servicemultiple.yaml -n test-keepalived-operator
oc apply -f ./test/keepalivedgroup2.yaml -n test-keepalived-operator
oc apply -f ./test/test-service-g2.yaml -n test-keepalived-operator
```

## Release Process

To release execute the following:

```shell
git tag -a "<version>" -m "release <version>"
git push upstream <version>
```

use this version format: vM.m.z
