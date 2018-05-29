# event-reporter

`event-reporter` exposes a simple API over D-BUS which it then stores
in a simple embedded database. Queued events are periodically sent to
the Cacophony Project API server.

This software is licensed under the GNU General Public License v3.0.

## D-BUS API

`event-reporter` exposes an API at "org.cacophony.Events" on the D-BUS
system bus. It a exposes a single method with the following signature:

```
Queue(details []byte, nanos int64) error
```

The `details` argument should contain JSON encoded event details which
are compatible with the Cacophony Project [events POST
API](https://api.cacophony.org.nz/#api-Events-Add_Event). The `nanos`
argument is the timestamp for the event in number of nanoseconds since
1970-01-01 UTC.

Events sent to this API will be persisted to disk and sent by
event-reporter at its next send interval.

Here's an example of how to send to the API using the `dbus-send` tool:

```
dbus-send --system --type=method_call --print-reply \
    --dest=org.cacophony.Events \
    /org/cacophony/Events \
    org.cacophony.Events.Queue \
    string:'{"description": {"type": "audioBait", "filename": "foo.mp3"}}' \
    int64:1527629858095250710
```

## Releases

This software uses the [GoReleaser](https://goreleaser.com) tool to
automate releases. To produce a release:

* Ensure that the `GITHUB_TOKEN` environment variable is set with a
  Github personal access token which allows access to the Cacophony
  Project repositories.
* Tag the release with an annotated tag. For example:
  `git tag -a "v1.4" -m "1.4 release"`
* Push the tag to Github: `git push --tags origin`
* Run `goreleaser --rm-dist`

The configuration for GoReleaser can be found in `.goreleaser.yml`.
