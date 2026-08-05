FROM scratch

COPY dist/console /console
COPY config/example-policy.yaml /repo/config/policy.yaml
COPY docs/reports /repo/docs/reports

USER 65532:65532
EXPOSE 8090
ENTRYPOINT ["/console", "--listen", "0.0.0.0:8090", "--policy", "/repo/config/policy.yaml", "--repository-root", "/repo"]
