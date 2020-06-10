/*

Remotes represent a (remote) cluster.

Remote
  |__ Watch -> Predicate,..,Router
  |__ Watch -> Predicate,..,Router
  |__ Watch -> Predicate,..,Router
  (router)
      |__Relay
      |    |__ Watch -> Predicate -> Forward -> Controller
      |    |__ Watch -> Predicate -> Forward -> Controller
      |    |__ Watch -> Predicate -> Forward -> Controller
      |
      |__Relay
           |__ Watch -> Predicate -> Forward -> Controller
           |__ Watch -> Predicate -> Forward -> Controller
           |__ Watch -> Predicate -> Forward -> Controller

Example:

//
// Create a remote (cluster).
remote := &watch.Remote{
    RestCfg: restCfg,
    Watch:
}

//
// Create a remote and watch resources.
remote := &watch.Remote{
    RestCfg: restCfg,
    Watch: []watch.Watch{
        {
            Object: &v1.Pod{},
            Predicates: []predicate{
                &predicate{},
            },
        },
        {
            Object: &v1.Secret{},
            Predicates: []predicate{
                &predicate{},
            },
        },
    }
}

//
// Create a remote and relay events to a controller.
remote := &watch.Remote{
    RestCfg: restCfg,
    Relay: []Relay{
        {
            Object: object,
            Controller: controller,
            Watch: []watch.Watch{
                {
                    Object: &v1.Pod{},
                    Predicates: []predicate{
                        &predicate{},
                    },
                },
                {
                    Object: &v1.Secret{},
                    Predicates: []predicate{
                        &predicate{},
                    },
                },
            }
        }
    }

//
// Start the remote.
remote.Start()

//
// Shutdown the remote.
remote.Shutdown()

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
