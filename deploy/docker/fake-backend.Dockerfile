FROM scratch

COPY dist/fake-backend /fake-backend

USER 65532:65532
EXPOSE 8000
ENTRYPOINT ["/fake-backend"]
