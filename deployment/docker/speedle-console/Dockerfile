FROM oraclelinux:7-slim
WORKDIR /speedle
COPY ./* /speedle/
EXPOSE 6735
ENV PATH="/speedle:${PATH}"

ENTRYPOINT ["entrypoint.sh"]
