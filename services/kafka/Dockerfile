FROM apache/kafka:latest

USER root
COPY services/kafka/entrypoint.sh /usr/local/bin/kafka-entrypoint.sh
RUN chmod 755 /usr/local/bin/kafka-entrypoint.sh && chown appuser:appuser /usr/local/bin/kafka-entrypoint.sh
USER appuser

ENTRYPOINT ["/usr/local/bin/kafka-entrypoint.sh"]
