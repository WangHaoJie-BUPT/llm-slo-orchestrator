FROM scratch

COPY dist/router /router
COPY config/e2e-fake-policy.yaml /config/policy.yaml

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/router"]
