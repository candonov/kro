# kro Nested RG example

This example creates a ResourceGroup called `WebStack` comprised of
three other RGs: `WebApp`, `S3Bucket`, and `PodIdentity`

### Create ResourceGroups

Change directory to `examples`:
```
cd examples/
```
Apply the RGs to your cluster:

```
kubectl apply -f podidenity/rg.yaml -f s3bucket/rg.yaml -f webapp/rg.yaml
kubectl apply -f webstack/rg.yaml
```

Validate the RGs statuses are Active:

```
kubectl get rg
```

Expected result:

```
NAME                  APIVERSION   KIND          STATE    AGE
podidentity.kro.run   v1alpha1     PodIdentity   Active    7m
s3bucket.kro.run      v1alpha1     S3Bucket      Active    7m
webapp.kro.run        v1alpha1     WebApp        Active    7m
webstack.kro.run      v1alpha1     WebStack      Active    7m
```

### Create an Instance of kind WebStack

Edit the spec.name in `webstack/instance.yaml`. The name of the spec will be the
name of your S3 bucket and S3 bucket should be unique, you can just add
a few random numbers to `test-app-11223344`.
```
vi webstack/instance.yaml
```
Apply the updated `webstack/instance.yaml` 

```
kubectl apply -f webstack/instance.yaml
```

Validate instance status:

```
kubectl get webstacks test-app
```

Expected result:

```
NAME       STATE    SYNCED   AGE
test-app   ACTIVE   True     16m
```

### Validate the app is working

Get the ingress url:

```
kubectl get ingress test-app-11223344 -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
```

Either navigate in the browser at `/health` or curl it:

```
curl -s $(kubectl get ingress test-app-11223344 -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')/health
```

Expected result:

```
{
  "message": "Application is running and can connect to S3",
  "status": "healthy"
}
```

### Troubleshoot
If you get the folling error:
```
Error connecting to S3:...
```
Try restarting the pod.

### Clean up

Remove the instance:

```
kubectl delete webstacks test-app
```

Remove the ResourceGroups:

```
kubectl delete rg webstack.kro.run webapp.kro.run s3bucket.kro.run podidentity.kro.run
```