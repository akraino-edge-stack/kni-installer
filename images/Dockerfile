FROM registry.redhat.io/rhel8/go-toolset
USER root
LABEL maintainer="Yolanda Robla <yroblamo@redhat.com>"

# install knictl
ENV GOPATH=$HOME/..
RUN mkdir -p gerrit.akraino.org/kni
WORKDIR $HOME/gerrit.akraino.org/kni
RUN git clone https://gerrit.akraino.org/r/kni/installer
WORKDIR $HOME/gerrit.akraino.org/kni/installer
RUN make build
RUN chmod a+x knictl
RUN mv knictl /usr/local/bin/
WORKDIR $HOME
