FROM flyway/flyway:11-alpine

USER root
RUN apk add --no-cache python3
COPY --chown=65532:65532 V*.sql /flyway/sql/
COPY --chown=65532:65532 run_migration.py /flyway/run_migration.py
RUN chmod 0555 /flyway/run_migration.py

WORKDIR /tmp
USER 65532:65532
ENTRYPOINT ["python3", "/flyway/run_migration.py"]
CMD ["migrate"]
