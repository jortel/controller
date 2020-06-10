/*

Remotes represent a (remote) cluster.

Remote
  |__ Watch -> Predicate, ...
  |__ Watch -> Predicate, ...
  |__ Watch -> Predicate, ...
  |__router
       |__ Relay
       |    |__ Watch -> Predicate,...,Forward -> Controller
       |    |__ Watch -> Predicate,...,Forward -> Controller
       |__ Relay
       |    |__ Watch -> Predicate,...,Forward -> Controller
       |    |__ Watch -> Predicate,...,Forward -> Controller
       |__ Relay
            |__ Watch -> Predicate,...,Forward -> Controller
            |__ Watch -> Predicate,...,Forward -> Controller

//
// Create a remote (cluster).
remote := &watch.Remote{
    RestCfg: restCfg,
}

//
// Add watch(s) and start the remote.
remote.Start(
    watch.Watch{
        Object: &v1.Pod{},
        Predicates: []predicate{
            &predicate{},
        },
    },
    watch.Watch{
        Object: &v1.Secret{},
        Predicates: []predicate{
            &predicate{},
        },
    })

//
// Create a relay and add to a remote.
remote.Relay(
    &watch.Relay{
        Object: object,
        Controller: controller,
        Watch: []watch.Watch{
            watch.Watch{
                Object: &v1.Pod{},
                Predicates: []predicate{
                    &predicate{},
                },
            },
            watch.Watch{
                Object: &v1.Secret{},
                Predicates: []predicate{
                    &predicate{},
                },
            },
        }
    })

//
// Shutdown the remote.
remote.Shutdown()

//
// Add individual watch.
remote.Watch(
    watch.Watch{
        Object: &v1.Secret{},
        Predicates: []predicate{
            &predicate{},
        },
    })

//
// Register your remote.
watch.Map.Add(myObject, remote)

//
// Find a registered remote.
remote, found := watch.Map.Find(myObject)

//
// Unregister a registered remote.
remote, found := watch.Map.Delete(myObject)
*/
package watch
