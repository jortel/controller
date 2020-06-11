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
// Add remote to the manager.
rmt.Manager.Add(owner, remote)

//
// Watch a resource using the manager.
rmt.Manager.EnsureWatch(
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
rmt.Manager.EnsureRelay(
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
rmt.Manager.EndRelay(
    owner,
    rmt.Relay{
        Controller: controller,
        Watch: rmt.Watch{
            Subject: &v1.Pod{},
        }
    })
*/
package remote
