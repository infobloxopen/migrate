#!/bin/bash

make build-docker
make build
docker tag infoblox/migrate:5.0.0-2-g8ab5a72-unsupported-j0 core-harbor-prod.sdp.infoblox.com/infobloxcto-dev/infoblox/migrate:5.0.0-2-g8ab5a72-unsupported-j0
docker push core-harbor-prod.sdp.infoblox.com/infobloxcto-dev/infoblox/migrate:5.0.0-2-g8ab5a72-unsupported-j0
docker run -it -v /Users/vvenkatasubramanian/go/src/github.com/Infoblox-CTO/ddi.keys/db/migrations/:/ns-migrations/ --net host --entrypoint /bin/sh core-harbor-prod.sdp.infoblox.com/infobloxcto-dev/infoblox/migrate:5.0.0-2-g8ab5a72-unsupported-j0
