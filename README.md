# event-reporter

`event-reporter` exposes a simple API over D-BUS which it then stores
in a simple embedded database. It also has a GO library (eventclient) to help with this. Queued events are periodically sent to
the Cacophony Project API server.

Events are also made to work with the sidekick app so events can be collected from remote locations. The API for this is done through the [management-interface](https://github.com/TheCacophonyProject/management-interface)


## D-BUS API

`event-reporter` exposes an API at "org.cacophony.Events" on the D-BUS
system bus. It a exposes several methods for adding, getting, and deleting events.

### Add
Adding a new event
```
Add(details string, eventType, unixNsec int64) error
```
- `details` JSON encoded event details which
are compatible with the Cacophony Project [events POST
API](https://api.cacophony.org.nz/#api-Events-Add_Event).
- `eventType` Type of event
- `unixNsec` Unix timestamp in nanoseconds.

```
dbus-send --system --type=method_call --print-reply \
     --dest=org.cacophony.Events /org/cacophony/Events \
     org.cacophony.Events.Add \
     string:'{"foo":"bar"}' \
     string:test \
     int64:1527629858095250710
```

### GetKeys
Get list of all event keys
```
GetKeys() ([]uint64, error)
```


### Get
Get details of one event
```
Get(key uint64) error
```
- `key` Key of event that you want to get.

### Delete
Delete an event from the event store.
```
Delete(key uint64) error
```
- `key` Key of event that you want to delete.

## Event Client
If using go use the eventclient for interfacing with the API instead of making dbus calls. This has `AddEvent`, `GetEventKeys`, `GetEvent`, and `DeleteEvent`

## Releases

Releases are built using TravisCI. To create a release visit the
[repository on Github](https://github.com/TheCacophonyProject/audiobait/releases)
and then follow our [general instructions](https://docs.cacophony.org.nz/home/creating-releases)
for creating a release.

## License

This software is licensed under the GNU General Public License v3.0.
