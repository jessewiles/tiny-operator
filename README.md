# tiny-operator
🎶 Hold me close you tiny operator 🎶

The purpose of this project is to provide the initial operator strapping to allow rapid testing of
CRD deployment and other k8s object creation outside of a dedicated operator.  All of the operator code
(that isn't tied up with code generation) is located in `cmd/operator/main.go`.  Modify the `createCRD`,
`onApplyEcho` and `onDeleteEcho` functions to run quick minikube tests on how k8s will handle
a CRD and the associated objects it handles (replicasets, deployments, statefulsets, etc.)

## Running the operator

 1. Checkout the code.
 1. `dep ensure` the dependencies.
 1. Run the operator (at minikube)

    ```
    $ go run cmd/operator/main.go --kubecfg-file=${HOME}/.kube/config
    ```
