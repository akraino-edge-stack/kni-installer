#!/bin/sh

set +x

prerequisites()
{
    # Check if virtualization is supported
    ls /dev/kvm 2> /dev/null
    if [ 0 -ne 0 ]
    then
        echo "Your system doesn't support virtualization"
        exit 1
    fi

    # Install required dependecies
    sudo yum install -y libvirt libvirt-devel libvirt-daemon-kvm qemu-kvm

    # Enable IP forwarding
    sudo sysctl net.ipv4.ip_forward=1

    # Configure libvirt to accept TCP connections
    sudo sed -i.bak -e 's/^[#]*\s*listen_tls.*/listen_tls = 0/' -e 's/^[#]*\s*listen_tcp.*/listen_tcp = 1/' -e 's/^[#]*\s*auth_tcp.*/auth_tcp = "none"/' -e 's/^[#]*\s*tcp_port.*/tcp_port = "16509"/' /etc/libvirt/libvirtd.conf

    # Configure the service runner to pass --listen to libvirtd
    sudo sed -i.bak -e 's/^[#]*\s*LIBVIRTD_ARGS.*/LIBVIRTD_ARGS="--listen"/' /etc/sysconfig/libvirtd

    # Restart the libvirtd service
    sudo systemctl restart libvirtd

    # Get active Firewall zone option
    systemctl is-active firewalld
    if [ 0 -ne 0 ]
    then
        echo "Your system doesn't have firewalld service running"
        exit 1
    fi

    activeZone=FedoraWorkstation
    sudo firewall-cmd --zone= --add-source=192.168.126.0/24
    sudo firewall-cmd --zone= --add-port=16509/tcp

    # Configure default libvirt storage pool
    sudo virsh --connect qemu:///system pool-list | grep -q 'default'
    if [ 0 -ne 0 ]
    then
        sudo virsh pool-define /dev/stdin <<EOF
<pool type='dir'>
  <name>default</name>
  <target>
    <path>/var/lib/libvirt/images</path>
  </target>
</pool>
EOF

    sudo virsh pool-start default
    sudo virsh pool-autostart default
    fi

    # Set up NetworkManager DNS overlay
    dnsconf=/etc/NetworkManager/conf.d/crc-libvirt-dnsmasq.conf
    local dnschanged=""
    if ! [ -f "" ]; then
        echo -e "[main]\ndns=dnsmasq" | sudo tee ""
        dnschanged=1
    fi
    dnsmasqconf=/etc/NetworkManager/dnsmasq.d/openshift.conf
    if ! [ -f "" ]; then
        echo server=/tt.testing/192.168.126.1 | sudo tee ""
        echo address=/apps.test.tt.testing/192.168.126.11 | sudo tee -a ""
        dnschanged=1
    fi
    if [ -n "" ]; then
        sudo systemctl restart NetworkManager
    fi

    # Create an entry in the /etc/host
    grep -q 'libvirt.default' /etc/hosts
    if [ 0 -ne 0 ]
    then
        echo '192.168.126.1   libvirt.default' | sudo tee --append /etc/hosts
    fi
}

prerequisites
