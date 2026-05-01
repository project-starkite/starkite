#!/usr/bin/env cloudkite
# leader-election.star — Demonstrates leader election for HA controllers
#
# Run two copies in separate terminals to see leader election in action:
#   Terminal 1: cloudkite run leader-election.star --var id=replica-1
#   Terminal 2: cloudkite run leader-election.star --var id=replica-2
#
# Only one will print reconcile messages (the leader).
# Kill the leader — the other takes over within ~15 seconds.

replica_id = var_str("id", "default")
printf("Starting replica %s...\n", replica_id)

def on_create(obj):
    printf("[%s] Created: %s/%s\n", replica_id, obj.metadata.namespace, obj.metadata.name)

def on_delete(obj):
    printf("[%s] Deleted: %s/%s\n", replica_id, obj.metadata.namespace, obj.metadata.name)

k8s.control("configmaps",
    on_create = on_create,
    on_delete = on_delete,
    namespace = "default",
    leader_election = True,
    leader_election_id = "configmap-leader",
)
