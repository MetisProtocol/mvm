# l2geth build metadata

Run from the repository root to embed the checked-out commit and its UTC date:

```sh
export GIT_COMMIT="$(git rev-parse HEAD)"
export GIT_DATE="$(TZ=UTC git show -s --format=%cd --date=format-local:%Y%m%d HEAD)"
docker build -f ops/docker/Dockerfile.geth \
  --build-arg GIT_COMMIT="$GIT_COMMIT" \
  --build-arg GIT_DATE="$GIT_DATE" \
  -t l2geth:local .
docker run --rm --entrypoint geth l2geth:local version
```

Both `docker-compose.yml` and `ops/docker-compose-build.yml` pass these exported
variables to the l2geth build. The l2geth image workflow supplies them automatically
from the checkout. Docker builds without these optional arguments still succeed,
but omit Git metadata because the builder contains no `.git` directory.

`GIT_DATE` uses `YYYYMMDD`, not a Unix timestamp. Use the commit date, not the build
date. Local builds also discover Git metadata from the enclosing repository.
Explicit `build/ci.go install -git-commit ... -git-date ...` flags override the
environment variables, which override automatic Git detection. Source archives
can use the same flags or environment variables without a Git checkout.
