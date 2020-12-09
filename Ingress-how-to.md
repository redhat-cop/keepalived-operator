# Ingress How To

This document explains who to configure the OpenShift ingress controller to take advantage of the keepalived-operator.

## Ingress configuration

Assuming you have a properly installed keeapalived configuration, proceed as follows:

Create an ingress controller:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: my-keepalived-ingress
  namespace: openshift-ingress-operator
spec:
  domain: myingress.mydomain
  replicas: 2
  endpointPublishingStrategy: 
    type: Private
```

You can add any other field needed to your configuration, the important thing here is `endpointPublishingStrategy: Private`.
This will create a set of pods in teh openshift-ingress namespace with prefix: `router-my-keepalived-ingress`.

Create a load balancer service to server these pods:

```yaml
kind: Service
apiVersion: v1
metadata:
  annotations:
    keepalived-operator.redhat-cop.io/keepalivedgroup: <keepalived-group>
  name: router-my-keepalived-ingress
  namespace: openshift-ingress
spec:
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: http
    - name: https
      protocol: TCP
      port: 443
      targetPort: https
  selector:
    ingresscontroller.operator.openshift.io/deployment-ingresscontroller: my-keepalived-ingress
  type: LoadBalancer
```

At this point the keepalievd operator will provision a VIP and the routers are reachable there.

If you need to control which IP the router needs to be serve on, use an external IPs:

```yaml
kind: Service
apiVersion: v1
metadata:
  annotations:
    keepalived-operator.redhat-cop.io/keepalivedgroup: <keepalived-group>
  name: router-my-keepalived-ingress
  namespace: openshift-ingress
spec:
  externalIPs:
    - <external IP>
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: http
    - name: https
      protocol: TCP
      port: 443
      targetPort: https
  selector:
    ingresscontroller.operator.openshift.io/deployment-ingresscontroller: my-keepalived-ingress
  type: ClusterIP
```

At this point the only missing ingredient is to make sure that the DNS routes requests to `*.myingress.mydomain` are directed to the IP that was provisioned by the keepalived-operator or that was selected via the external IP.

If you are using the [external-dns](https://github.com/kubernetes-sigs/external-dns) operator, you can easily automate this step by adding the following annotation to the service: `external-dns.alpha.kubernetes.io/hostname: *.myingress.mydomain`.
