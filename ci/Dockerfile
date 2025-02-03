FROM debian:testing-slim
RUN apt-get -y update && \
    apt-get -y install golang dbus git && \
    apt-get -y upgrade && \
    apt-get -y clean && \
    rm -rf /var/cache/apt/*
