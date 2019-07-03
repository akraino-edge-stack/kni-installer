#!/bin/bash

#set -e

###------------------------------------------------###
### Need interface input from user via environment ###
###------------------------------------------------###

printf "\nChecking parameters...\n\n"

for i in PROV_INTF BM_INTF EXT_INTF BSTRAP_BM_MAC MASTER_BM_MAC WORKER_BM_MAC PULL_SECRET_FILE; do
    if [[ -z "${!i}" ]]; then
        echo "You must set PROV_INTF, BM_INTF, EXT_INTF, BSTRAP_BM_MAC, MASTER_BM_MAC, WORKER_BM_MAC and PULL_SECRET_FILE as environment variables!"
        exit 1
    else
        echo $i": "${!i}
    fi
done

###----------------------------------###
### Configure provisioning interface ###
###----------------------------------###

printf "\nConfiguring provisioning interface ($PROV_INTF)...\n\n"

cat <<EOF > /etc/sysconfig/network-scripts/ifcfg-$PROV_INTF
TYPE=Ethernet
PROXY_METHOD=none
BROWSER_ONLY=no
BOOTPROTO=static
DEFROUTE=yes
IPV4_FAILURE_FATAL=no
NAME=$PROV_INTF
DEVICE=$PROV_INTF
ONBOOT=yes
IPADDR=172.22.0.10
NETMASK=255.255.255.0
EOF

ifdown $PROV_INTF
ifup $PROV_INTF

###-------------------------------###
### Configure baremetal interface ###
###-------------------------------###

printf "\nConfiguring baremetal interface ($BM_INTF)...\n\n"

cat <<EOF > /etc/sysconfig/network-scripts/ifcfg-$BM_INTF
TYPE=Ethernet
NM_CONTROLLED=no
PROXY_METHOD=none
BROWSER_ONLY=no
BOOTPROTO=static
DEFROUTE=no
IPV4_FAILURE_FATAL=no
IPV6INIT=yes
IPV6_AUTOCONF=yes
IPV6_DEFROUTE=yes
IPV6_FAILURE_FATAL=no
IPV6_ADDR_GEN_MODE=stable-privacy
NAME=$BM_INTF
DEVICE=$BM_INTF
IPADDR=192.168.111.1
NETMASK=255.255.255.0
ONBOOT=yes
EOF

ifdown $BM_INTF
ifup $BM_INTF

###-----------------------------###
### Create required directories ###
###-----------------------------###

printf "\nCreating required directories...\n\n"

if [[ ! -d "~/dev/test1" ]]; then
    mkdir -p ~/dev/test1
    mkdir -p ~/dev/upi-dnsmasq/$PROV_INTF
    mkdir -p ~/dev/upi-dnsmasq/$BM_INTF
    mkdir -p ~/dev/scripts
    mkdir -p ~/dev/containers/haproxy
    sudo mkdir -p /etc/matchbox
    mkdir -p ~/.matchbox
    sudo mkdir -p /var/lib/matchbox/assets
    sudo mkdir -p /etc/coredns
    sudo mkdir -p /var/run/dnsmasq
    sudo mkdir -p /var/run/dnsmasq2
    mkdir -p ~/go/src
fi

###--------------------------------------------------###
### Configure iptables to allow for external traffic ###
###--------------------------------------------------###

printf "\nConfiguring iptables to allow for external traffic...\n\n"

cat <<EOF > ~/dev/scripts/iptables.sh
#!/bin/bash

ins_del_rule()
{
    operation=\$1
    table=\$2
    rule=\$3
   
    if [ "\$operation" == "INSERT" ]; then
        if ! sudo iptables -t "\$table" -C \$rule > /dev/null 2>&1; then
            sudo iptables -t "\$table" -I \$rule
        fi
    elif [ "\$operation" == "DELETE" ]; then
        sudo iptables -t "\$table" -D \$rule
    else
        echo "\${FUNCNAME[0]}: Invalid operation: \$operation"
        exit 1
    fi
}

    #allow DNS/DHCP traffic to dnsmasq
    ins_del_rule "INSERT" "filter" "INPUT -i $BM_INTF -p udp -m udp --dport 67 -j ACCEPT"
    ins_del_rule "INSERT" "filter" "INPUT -i $BM_INTF -p udp -m udp --dport 53 -j ACCEPT"
   
    #enable routing from cluster network to external
    ins_del_rule "INSERT" "nat" "POSTROUTING -o $EXT_INTF -j MASQUERADE"
    ins_del_rule "INSERT" "filter" "FORWARD -i $PROV_INTF -o $EXT_INTF -j ACCEPT"
    ins_del_rule "INSERT" "filter" "FORWARD -o $PROV_INTF -i $EXT_INTF -m state --state RELATED,ESTABLISHED -j ACCEPT"
EOF

pushd ~/dev/scripts
chmod 755 iptables.sh
./iptables.sh
popd

###------------------------------------------###
### Install Git, Podman, Unzip, and Ipmitool ###
###------------------------------------------###

printf "\nInstalling dependencies via yum...\n\n"

sudo yum install -y git podman unzip ipmitool

###----------------###
### Install Golang ###
###----------------###

printf "\nInstalling Golang...\n\n"

if [[ ! -d "/usr/local/go" ]]; then
    pushd /tmp
    curl -O https://dl.google.com/go/go1.12.6.linux-amd64.tar.gz
    tar -xzf go1.12.6.linux-amd64.tar.gz
    sudo mv go /usr/local
    export GOROOT=/usr/local/go
    export GOPATH=$HOME/go/src
    export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
    # TODO: Use sed instead below?
    echo "export GOROOT=/usr/local/go" >> ~/.bash_profile
    echo "export GOPATH=$HOME/go/src" >> ~/.bash_profile
    echo "export PATH=$GOPATH/bin:$GOROOT/bin:$PATH" >> ~/.bash_profile
    popd
fi

###-----------------###
### Set up tftpboot ###
###-----------------###

# TODO: This might be unnecessary, as the dnsmasq container images we
#       are using are rumored to self-contain this
printf "\nSetting up tftpboot...\n\n"

if [[ ! -d "/var/lib/tftpboot" ]]; then
    mkdir -p /var/lib/tftpboot
    pushd /var/lib/tftpboot
    curl -O http://boot.ipxe.org/ipxe.efi
    curl -O http://boot.ipxe.org/undionly.kpxe
    popd
fi

###-----------------------------------------###
### Create HAProxy configuration and assets ###
###-----------------------------------------###

# TODO: Check if image is already built, or pre-build it elsewhere
#       and remove this section completely
printf "\nConfiguring HAProxy and building container image...\n\n"

HAPROXY_IMAGE_ID=`podman images | grep akraino-haproxy | awk {'print $3'}`

if [[ -z "$HAPROXY_IMAGE_ID" ]]; then
    pushd ~/dev/containers/haproxy

cat <<EOF > haproxy.cfg
#---------------------------------------------------------------------
# Global settings
#---------------------------------------------------------------------
global
    log         127.0.0.1 local2

    chroot      /var/lib/haproxy
    pidfile     /var/run/haproxy.pid
    maxconn     4000
    user        haproxy
    group       haproxy
    daemon

    # turn on stats unix socket
    stats socket /var/lib/haproxy/stats

#---------------------------------------------------------------------
# common defaults that all the 'listen' and 'backend' sections will
# use if not designated in their block
#---------------------------------------------------------------------
defaults
    mode                    http
    log                     global
    option                  httplog
    option                  dontlognull
    option forwardfor       except 127.0.0.0/8
    option                  redispatch
    retries                 3
    timeout http-request    10s
    timeout queue           1m
    timeout connect         10s
    timeout client          1m
    timeout server          1m
    timeout http-keep-alive 10s
    timeout check           10s
    maxconn                 3000

frontend kubeapi
    mode tcp
    bind *:6443
    option tcplog
    default_backend kubeapi-main

frontend mcs
    bind *:22623
    default_backend mcs-main
    mode tcp
    option tcplog

frontend http
    bind *:80
    mode tcp
    default_backend http-main
    option tcplog

frontend https
    bind *:443
    mode tcp
    default_backend https-main
    option tcplog

backend kubeapi-main
    balance source
    mode tcp
    server bootstrap 192.168.111.10:6443 check
    server master-0  192.168.111.11:6443 check

backend mcs-main
    balance source
    mode tcp
    server bootstrap 192.168.111.10:22623 check
    server master-0  192.168.111.11:22623 check

backend http-main
    balance source
    mode tcp
    server worker-0  192.168.111.50:80 check

backend https-main
    balance source
    mode tcp
    server worker-0  192.168.111.50:443 check
EOF

cat <<'EOF' > Dockerfile
FROM haproxy:1.7
COPY haproxy.cfg /usr/local/etc/haproxy/haproxy.cfg

ENV HAPROXY_USER haproxy

EXPOSE 80
EXPOSE 443
EXPOSE 6443
EXPOSE 22623

RUN groupadd --system ${HAPROXY_USER} && \
useradd --system --gid ${HAPROXY_USER} ${HAPROXY_USER} && \
mkdir --parents /var/lib/${HAPROXY_USER} && \
chown -R ${HAPROXY_USER}:${HAPROXY_USER} /var/lib/${HAPROXY_USER}

CMD ["haproxy", "-db", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
EOF

HAPROXY_IMAGE_ID=`podman build . | rev | cut -d ' ' -f 1 | rev | tail -1`
podman tag $HAPROXY_IMAGE_ID akraino-haproxy:latest
popd
fi

###-------------------------###
### Start HAProxy container ###
###-------------------------###

# TODO: Check if container is already running
printf "\nStarting HAProxy container...\n\n"

HAPROXY_CONTAINER=`podman ps | grep haproxy`

if [[ -z "$HAPROXY_CONTAINER" ]]; then
    podman run -d --name haproxy --net=host -p 80:80 -p 443:443 -p 6443:6443 -p 22623:22623 $HAPROXY_IMAGE_ID -f /usr/local/etc/haproxy/haproxy.cfg
fi

###-------------------------------------------###
### Create provisioning dnsmasq configuration ###
###-------------------------------------------###

printf "\nConfiguring provisioning dnsmasq...\n\n"

cat <<EOF > ~/dev/upi-dnsmasq/$PROV_INTF/dnsmasq.conf
port=0 # do not activate nameserver
interface=$PROV_INTF
bind-interfaces

dhcp-range=172.22.0.11,172.22.0.30,30m

# do not send default route
dhcp-option=3
dhcp-option=6

# Legacy PXE
dhcp-match=set:bios,option:client-arch,0
dhcp-boot=tag:bios,undionly.kpxe

# UEFI
dhcp-match=set:efi32,option:client-arch,6
dhcp-boot=tag:efi32,ipxe.efi
dhcp-match=set:efibc,option:client-arch,7
dhcp-boot=tag:efibc,ipxe.efi
dhcp-match=set:efi64,option:client-arch,9
dhcp-boot=tag:efi64,ipxe.efi

# verbose
log-queries
log-dhcp

dhcp-leasefile=/var/run/dnsmasq/$PROV_INTF.leasefile
log-facility=/var/run/dnsmasq/$PROV_INTF.log

# iPXE - chainload to matchbox ipxe boot script
dhcp-userclass=set:ipxe,iPXE
dhcp-boot=tag:ipxe,http://172.22.0.10:8080/boot.ipxe

# Enable dnsmasq's built-in TFTP server
enable-tftp

# Set the root directory for files available via FTP.
tftp-root=/var/lib/tftpboot

tftp-no-blocksize

dhcp-boot=pxelinux.0

conf-dir=/etc/dnsmasq.d,.rpmnew,.rpmsave,.rpmorig

EOF

###----------------------------------------###
### Create baremetal dnsmasq configuration ###
###----------------------------------------###

printf "\nConfiguring baremetal dnsmasq...\n\n"

cat <<EOF > ~/dev/upi-dnsmasq/$BM_INTF/dnsmasq.conf
port=0
interface=$BM_INTF
bind-interfaces

strict-order
pid-file=/var/run/dnsmasq/$BM_INTF.pid
except-interface=lo

dhcp-range=192.168.111.10,192.168.111.60,30m
#default gateway
dhcp-option=3,192.168.111.1
#dns server
dhcp-option=6,192.168.111.1

log-queries
log-dhcp

dhcp-no-override
dhcp-authoritative
dhcp-hostsfile=/var/run/dnsmasq/$BM_INTF.hostsfile

dhcp-leasefile=/var/run/dnsmasq/$BM_INTF.leasefile
log-facility=/var/run/dnsmasq/$BM_INTF.log

EOF

cat <<EOF > /var/run/dnsmasq/$BM_INTF.hostsfile
$BSTRAP_BM_MAC,192.168.111.10,bootstrap
$MASTER_BM_MAC,192.168.111.11,master-0
$WORKER_BM_MAC,192.168.111.50,worker-1
EOF

###--------------------------------------###
### Start provisioning dnsmasq container ###
###--------------------------------------###

printf "\nStarting provisioning dnsmasq container...\n\n"

DNSMASQ_PROV_CONTAINER=`podman ps | grep dnsmasq-prov`

if [[ -z "$DNSMASQ_PROV_CONTAINER" ]]; then
    podman run -d --name dnsmasq-prov --net=host -v /var/run/dnsmasq:/var/run/dnsmasq:Z \
    -v ~/dev/upi-dnsmasq/$PROV_INTF:/etc/dnsmasq.d:Z \
    --expose=53 --expose=53/udp --expose=67 --expose=67/udp --expose=69 --expose=69/udp \
    --cap-add=NET_ADMIN quay.io/poseidon/dnsmasq --conf-file=/etc/dnsmasq.d/dnsmasq.conf -u root -d -q
fi

###-----------------------------------###
### Start baremetal dnsmasq container ###
###-----------------------------------###

printf "\nStarting baremetal dnsmasq container...\n\n"

DNSMASQ_BM_CONTAINER=`podman ps | grep dnsmasq-bm`

if [[ -z "$DNSMASQ_BM_CONTAINER" ]]; then
    podman run -d --name dnsmasq-bm --net=host -v /var/run/dnsmasq2:/var/run/dnsmasq:Z \
    -v ~/dev/upi-dnsmasq/$BM_INTF:/etc/dnsmasq.d:Z \
    --expose=53 --expose=53/udp --expose=67 --expose=67/udp --expose=69 --expose=69/udp \
    --cap-add=NET_ADMIN quay.io/poseidon/dnsmasq --conf-file=/etc/dnsmasq.d/dnsmasq.conf -u root -d -q
fi

###--------------------###
### Configure matchbox ###
###--------------------###

printf "\nConfiguring matchbox...\n\n"

pushd ~/dev/containers

if [[ ! -d "matchbox" ]]; then
    git clone https://github.com/poseidon/matchbox.git
    pushd matchbox/scripts/tls
    export SAN=IP.1:172.22.0.10
    ./cert-gen
    sudo cp ca.crt server.crt server.key /etc/matchbox
    cp ca.crt client.crt client.key ~/.matchbox 
    popd
fi

popd

###--------------------------###
### Start matchbox container ###
###--------------------------###

printf "\nStarting matchbox container...\n\n"

MATCHBOX_CONTAINER=`podman ps | grep matchbox`

if [[ -z "$MATCHBOX_CONTAINER" ]]; then
    podman run -d --net=host --name matchbox -v /var/lib/matchbox:/var/lib/matchbox:Z -v /etc/matchbox:/etc/matchbox:Z,ro quay.io/poseidon/matchbox:latest -address=0.0.0.0:8080 -rpc-address=0.0.0.0:8081 -log-level=debug
fi

###----------------------------------------###
### Configure coredns Corefile and db file ###
###----------------------------------------###

printf "\nConfiguring CoreDNS...\n\n"

if [[ ! -f "/etc/coredns/Corefile" ]]; then
cat <<EOF > /etc/coredns/Corefile
.:53 {
    log
    errors
    forward . 10.11.5.19
}

tt.testing:53 {
    log
    errors
    file /etc/coredns/db.tt.testing
    debug
}

EOF

cat <<'EOF' > /etc/coredns/db.tt.testing
$ORIGIN tt.testing.
$TTL 10800      ; 3 hours
@       3600 IN SOA sns.dns.icann.org. noc.dns.icann.org. (
                                2019010101 ; serial
                                7200       ; refresh (2 hours)
                                3600       ; retry (1 hour)
                                1209600    ; expire (2 weeks)
                                3600       ; minimum (1 hour)
                                )

_etcd-server-ssl._tcp.test1.tt.testing. 8640 IN    SRV 0 10 2380 etcd-0.test1.tt.testing.

api.test1.tt.testing.                        A 192.168.111.1
api-int.test1.tt.testing.                    A 192.168.111.1
test1-master-0.tt.testing.                   A 192.168.111.11
test1-worker-0.tt.testing.                   A 192.168.111.50
test1-bootstrap.tt.testing.                  A 192.168.111.10
etcd-0.test1.tt.testing.                     IN  CNAME test1-master-0.tt.testing.

$ORIGIN apps.test1.tt.testing.
*                                                    A                192.168.111.1
EOF
fi

###-------------------------###
### Start coredns container ###
###-------------------------###

printf "\nStarting CoreDNS container...\n\n"

COREDNS_CONTAINER=`podman ps | grep coredns`

if [[ -z "$COREDNS_CONTAINER" ]]; then
    podman run -d --expose=53 --expose=53/udp -p 192.168.111.1:53:53 -p 192.168.111.1:53:53/udp \
    -v /etc/coredns:/etc/coredns:z --name coredns coredns/coredns:latest -conf /etc/coredns/Corefile
fi

###----------------------------###
### Prepare OpenShift binaries ###
###----------------------------###

printf "\nInstalling OpenShift binaries...\n\n"

pushd /tmp

if [[ ! -f "/usr/local/bin/openshift-install" ]]; then
    # TODO: These versions change without warning!  Need to accomodate for this.
    curl -O https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-install-linux-4.1.4.tar.gz
    tar xvf openshift-install-linux-4.1.4.tar.gz
    sudo mv openshift-install /usr/local/bin/
    curl -O https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-client-linux-4.1.4.tar.gz
    tar xvf openshift-client-linux-4.1.4.tar.gz
    sudo mv oc /usr/local/bin/
fi

###-------------------###
### Prepare terraform ###
###-------------------###

printf "\nInstalling Terraform...\n\n"

if [[ ! -f "/usr/bin/terraform" ]]; then
    curl -O https://releases.hashicorp.com/terraform/0.12.2/terraform_0.12.2_linux_amd64.zip
    unzip terraform_0.12.2_linux_amd64.zip
    sudo mv terraform /usr/bin/.
    git clone https://github.com/poseidon/terraform-provider-matchbox.git
    go build
    mkdir -p ~/.terraform.d/plugins
    cp terraform-provider-matchbox ~/.terraform.d/plugins/.
fi

popd

printf "\nDONE\n"