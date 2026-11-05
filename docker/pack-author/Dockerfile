# pack-author: the sandbox a pack's source is generated and checked in (C6) — the runtimes the SDK's
# generators run on: Go, Python, a JDK. A checkout is mounted at /work:
#   docker build -t horchestra/pack-author docker/pack-author
#   docker run --rm -v <checkout>:/work horchestra/pack-author go run . generate -o out
FROM golang:1.25-bookworm
RUN apt-get update \
    && apt-get install -y --no-install-recommends python3 python3-pip default-jdk-headless git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /work
ENTRYPOINT ["/bin/sh", "-c"]
