/*

Remotes represent a (remote) cluster.

Remote
  |__ Watch -> Predicate,..,Forward [F]
  |__ Watch -> Predicate,..,Forward [F]
  |__ Watch -> Predicate,..,Forward [F]
  |__ *
.
  [F] Forward ->|
                |_Relay -> channel -> (watch)Controller
                |_Relay -> channel -> (watch)Controller
                |_Relay -> channel -> (watch)Controller
                |_*

Example:
_______________________________

import (
    rmt ".../remote"
)

//
// Create a remote (cluster).
remote := &rmt.Remote{
    RestCfg: restCfg,
}

//
// Start the remote.
remote.Start()

//
// Watch a resource.
remote.EnsureWatch(
    rmt.Watch{
        Subject: &v1.Pod{},
        Predicates: []predicate{
                &predicate{},
            },
        }
    })

//
// Watch a resource and relay events to a controller.
remote.EnsureRelay(
    rmt.Relay{
        Controller: controller,
        Target: target,
        Watch: rmt.Watch{
            Subject: &v1.Pod{},
            Predicates: []predicate{
                    &predicate{},
                },
            }
        }
    })

//
// End a relay.
remote.EndRelay(
    rmt.Relay{
        Controller: controller,
        Watch: rmt.Watch{
            Subject: &v1.Pod{},
        }
    })

//
// Shutdown a remote.
remote.Shutdown()

//
// Add remote to the remote container.
rmt.Container.Add(owner, remote)

//
// Ensure remote is in the remote container
// and started with this configuration.
rmt.Container.Ensure(owner, remote)

//
// Watch a resource using the remote container.
rmt.Container.EnsureWatch(
    owner,
    rmt.Watch{
        Subject: &v1.Pod{},
        Predicates: []predicate{
                &predicate{},
            },
        }
    })

//
// Watch a resource and relay events to a controller.
rmt.Container.EnsureRelay(
    owner,
    rmt.Relay{
        Controller: controller,
        Target: target,
        Watch: rmt.Watch{
            Subject: &v1.Pod{},
            Predicates: []predicate{
                    &predicate{},
                },
            }
        }
    })

//
// End a relay.
rmt.Container.EndRelay(
    owner,
    rmt.Relay{
        Controller: controller,
        Watch: rmt.Watch{
            Subject: &v1.Pod{},
        }
    })

//
// Ensure a RelayDefinition.

def := &RelayDefinition{
	Channel: aChannel,
	Target: target,
	Watch: []WatchDefinition{
		{
			RemoteOwner: nil, // source cluster
			Watch: []Watch{
				{
					Subject: &v1.Pod{},
					Predicates: []predicate.Predicate{
						&predicate{},
					},
				},
			},
		},
		{
			RemoteOwner: nil, // destination cluster
			Watch: []Watch{
				{
					Subject: &v1.Pod{},
					Predicates: []predicate.Predicate{
						&predicate{},
					},
				},
			},
		},
	},
}
rmt.Container.EnsureRelayDefinition(def)

_______________________________

Example Pattern:

Cluster (CR) and controller.
  The Cluster represents a real cluster.

PodCounter (CR) and controller that keeps a count of pods on remote clusters.
  The PodCounter (CR) has a reference to a `Cluster`.

Cluster reconcile:
  (created/update)
    1. Get cluster.
    2. Ensure a `Remote` created, configured and started
       for the remote cluster.
  (deleted)
    1. Shutdown/delete the remote for the cluster.

PodCounter watch predicate.
  (created)
    1. Ensure (remote) watch/relay for `Pod` is established.
  (updated)
    1. End (remote) watch/relay on the previous remote.
    2. Ensure (remote) watch/relay for `Pod` is established
       on the new remote.
  (delete)
    1. End watch/relay on the remote.

*/
package remote
