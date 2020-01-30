# Keepalived operator

The objective of the keeplived operator provides is to allow for a way to create self-hosted load balancers in an automated way. From a user experience point of view the behavior is the same as of when creating [`LoadBalancer`](link)  services with a cloud provider able to manage them.

The keepalived operator can be used in all environments that allows nodes to advertise additional IPs on their NICs (and at least for now, in networks that allow multicast), however it's mainly aimed at supporting LoadBalancer services and ExternalIPs on baremetal installations (or other installation environments where a cloud provider is not available).

One possible use of the keepalived operator is also to support [OpenShift Ingress](link) in environments where an external load balancer cannot be provisioned.

## how it works

The keepalived operator will create one or more [VIPs](link) (an HA IP that floats between multiple nodes), based on the [LoadBalancer](link) services and/or services requesting [ExternalIPs](link).

For `LoadBalancer` services the IPs found at `.Status.LoadBalancer.Ingress[].IP` will become VIPs.

For services requesting an `externalIP`, the IPs found at `.Spec.ExternalIPs[]` will become VIPs.

Note that a service can be of LoadBalancer type and also request external IPs, it this case both sets of IPs will become VIPs.

Due to a keepalived limitation a single keepalived cluster can manage up to 256 VIP configurations. Multiple keepalived clusters can coexists in the same network as long as they use different multicast ports [TODO].

To address this limitation the KeepalivedGroup CRD has been introduced. This CRD is supposed to be configured by an administrator and allows you to specify a node selector to pick on which nodes the keepalived pods should be deployed. Here is an example:

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

This keepalived group will be deployed on all the nodes with role `loadbalancer`. One must also specify the network device on which the VIPs will be exposed, it is assumed that all the nodes have the same network device configuration.

Services must be annotated to opt-in being observed by the keepalived operator and to specify which keepalived group they refer to, the annotation looks like this:
`keepalived-operator.redhat-cop.io/keepalivedgroup: <keepalivedgreoup namespace>/<keepalivedgroup-name>`

## Requirements

Each keepalived group deploys a [daemonset](link) that requires the privileged scc, this permission must be given to the `default` service account in the namespace where the keepalived group is created by and administrator [TODO, use pcc and automate it]

For OpenShift user only it is necessary to allow for loadbalancer ips to be automatically assigned by the systems and to external IPs to be selected by the users. This can be done by patching the cluster network. Here is an example of the patch:

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

## Metrics collection

Each keepalived pod expose a [Prometheus](link) metrics port at `9650`. When a keepalived group is created a [`PodMonitor`](link) rule to collect those metrics. It is up to you to make sure your Prometheus instance watches for those `PodMonitor` rules.

## Deploying the Operator

This is a cluster-level operator that you can deploy in any namespace, `keepalived-operator` is recommended.

You can either deploy it using [`Helm`](https://helm.sh/) or creating the manifests directly.

### Deploying with Helm

Here are the instructions to install the latest release with Helm.

```shell
oc new-project keepalived-operator

helm repo add keepalived-operator https://redhat-cop.github.io/keepalived-operator
helm repo update
export must_gather_operator_chart_version=$(helm search keepalived-operator/keepalived-operator | grep keepalived-operator/keepalived-operator | awk '{print $2}')

helm fetch keepalived-operator/keepalived-operator --version ${must_gather_operator_chart_version}
helm template keepalived-operator-${must_gather_operator_chart_version}.tgz --namespace keepalived-operator | oc apply -f - -n keepalived-operator

rm keepalived-operator-${must_gather_operator_chart_version}.tgz
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
oc annotate svc django-psql-example -n test-keepalived-operator cert-utils-operator.redhat-cop.io/KeepalivedGroup=test-keepalived-operator/keepalivedgroup-test
```

curl the app using the service IP

```shell
curl http://$SERVICE_IP:8080
```