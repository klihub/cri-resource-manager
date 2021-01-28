# make sure there's no pod1 in kube-system from the previous run
kubectl delete pod pod1 -n kube-system

# pod0: 4 containers, one per socket, each container taking > 50 % of socket's mem
CPU=200m MEM=5G CONTCOUNT=4 create guaranteed
report allowed

# pod1: reserve neglible amount of memory...
# first from a socket:
CPU=200m MEM=100M create guaranteed
report allowed
verify 'len(mems["pod1c0"]) == 2'

# second, from the root node.  FIX ME: extra hassle because old
# cri-resmgr e2e framework does not support creating directly to a
# namespace. But now we have pod1.yaml on VM. Let's put it in the
# kube-system, and get pod1c0 to the root node.
kubectl delete pods pod1 --now
kubectl create -n kube-system -f pod1.yaml

vm-command "grep -A4 upward cri-resmgr.output.txt" && {
    report allowed
    pp mems
    error "upward raising detected! note that it may not match memory pinning..."
}

kubectl delete pod pod1 -n kube-system
