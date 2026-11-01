# Build binary first, then build the distroless image:
#   ./service/build-docker.sh
#   docker build -t zebragent:latest .
#
# Run container with shared host config (auto-restarts on system boot):
#   docker run -d --name zebragent --restart unless-stopped --network host -v ~/.config/zebragent:/config zebragent:latest
#
# Or run with Docker Compose:
#   docker compose up -d

FROM gcr.io/distroless/static-debian12

WORKDIR /

# Copy pre-compiled binary (built by service/build-docker.sh)
COPY service/zebragent /usr/local/bin/zebragent

ENV ZEBRA_CONFIG_PATH=/config/config.json
ENV XDG_CONFIG_HOME=/config

ENTRYPOINT ["/usr/local/bin/zebragent"]
