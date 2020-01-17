notes:
https://github.com/gen2brain/keepalived_exporter
https://godoc.org/github.com/rjeczalik/cmd/notify




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
make docker-push-latest
export KEEPALIVED_OPERATOR_IMAGE_NAME=${REPOSITORY}:latest
export KEEPALIVEDGROUP_TEMPLATE_FILE_NAME=./build/templates/keepalived-template.yaml
oc patch network cluster -p "$(cat ./test/externalIP-patch.yaml | yq -j .)" --type=merge
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