FROM mesosphere/kubernetes-mesos:latest
MAINTAINER Mesosphere <support@mesosphere.io>

RUN apt-get update -qq && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -qqy \
        apt-transport-https \
        ca-certificates \
        curl \
        lxc \
        iptables \
        ipcalc \
        && \
    apt-get clean

# Install latest Docker
# RUN curl -sSL https://get.docker.com/ubuntu/ | sh

# Install Docker 1.6.2 (docker 1.7 has regressions)
RUN echo deb https://get.docker.com/ubuntu docker main > /etc/apt/sources.list.d/docker.list
RUN apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
RUN apt-get update -qq && \
    DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -qqy \
        lxc-docker-1.6.2 \
        && \
    apt-get clean

# Install docker-in-docker wrapper
ADD ./bin/wrapdocker /usr/local/bin/wrapdocker
RUN chmod +x /usr/local/bin/wrapdocker

# Create volume where docker will mount loop devices
VOLUME /var/lib/docker

ENTRYPOINT ["wrapdocker"]
CMD ["mesos-slave"]
