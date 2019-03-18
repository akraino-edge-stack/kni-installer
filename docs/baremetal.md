## How to deploy on baremetal
At the moment the workflow for baremetal deployment is manual. It still needs to have a bootstrap VM to orchestrate the deployment, and some specific network configuration to interconnect the bootstrap VM and the baremetal servers.

**1. Get images**
In order to boot RHCOS on baremetal, PXE and raw images need to be used, it is not enough with having the qcow2 images. They can be produced internally using *make images* command, but they also can be consumed from published locations. Three images are going to be needed: vmlinuz, initrd and rhcos-qemu.raw.gz

**2. Setup network**
In order to deploy on baremetal, network needs to be configured manually. A bootstrap VM needs to be created, and this VM needs to have communication with the baremetal servers, using a shared network connection, bridge, etc...
We also assume that DHCP is configured properly to give IP addresses to baremetal servers.

**3. Generate artifacts**
First thing before starting the deployment is to retrieve the ignition files, that are going to be a set of ign files that are going to be used for the 3 different roles in openshift: bootstrap, master and worker.
In order to generate the artifacts, you need to have a binary of openshift-installer (compiled with libvirt support) , and run:

    openshift-install create ignition-configs

**4. Create cluster network and bootstrap VM**
First step will be to create the cluster network. It is going to be a libvirt network, but with some specific tweaks for DNS. The template will look like the following:

	<network>
	  <name>cluster</name>
	  <forward mode='nat'>
	    <nat>
	      <port start='1024' end='65535'/>
	    </nat>
	  </forward>
	  <bridge name='br0' stp='on' delay='0'/>
	  <mac address='52:54:00:b7:ec:0d'/>
	  <domain name='<cluster_name>.<cluster_domain>' localOnly='yes'/>
	  <dns>
	    <srv service='etcd-server-ssl' protocol='tcp' domain='<cluster_name>.<cluster_domain>' target='etcd-0.<cluster_name>.<cluster_domain>' port='2380' weight='10'/>
	    <srv service='etcd-server-ssl' protocol='tcp' domain='<cluster_name>.<cluster_domain>' target='etcd-1.<cluster_name>.<cluster_domain>' port='2380' weight='10'/>
	    <srv service='etcd-server-ssl' protocol='tcp' domain='<cluster_name>.<cluster_domain>' target='etcd-2.<cluster_name>.<cluster_domain>' port='2380' weight='10'/>
	    <host ip='192.168.126.2'>
	      <hostname>api.<cluster_name>.<cluster_domain></hostname>
	    </host>
	    <host ip='192.168.126.3'>
	      <hostname>api.<cluster_name>.<cluster_domain></hostname>
	      <hostname>etcd-0.<cluster_name>.<cluster_domain></hostname>
	    </host>
	    <host ip='192.168.126.4'>
	      <hostname>api.<cluster_name>.<cluster_domain></hostname>
	      <hostname>etcd-1.<cluster_name>.<cluster_domain></hostname>
	    </host>
	    <host ip='192.168.126.5'>
	      <hostname>api.<cluster_name>.<cluster_domain></hostname>
	      <hostname>etcd-2.<cluster_name>.<cluster_domain></hostname>
	    </host>
	    <host ip='192.168.126.0'>
	      <hostname><cluster_name>.<cluster_domain></hostname>
	    </host>
	  </dns>
	  <ip family='ipv4' address='192.168.126.1' netmask='255.255.255.0'>
	    <dhcp>
	      <range start='192.168.126.2' end='192.168.126.254'/>
	      <host mac='52:54:00:cc:cc:01' name='<cluster_name>-bootstrap' ip='192.168.126.2'/>
	      <host mac='52:54:00:cc:cc:02' name='<cluster_name>-master-0' ip='192.168.126.3'/>
	      <host mac='52:54:00:cc:cc:03' name='<cluster_name>-master-1' ip='192.168.126.4'/>
	      <host mac='52:54:00:cc:cc:04' name='<cluster_name>-master-2' ip='192.168.126.5'/>
	      <bootp file='bootstrap.ipxe' server='192.168.126.1'/>
	    </dhcp>
	  </ip>
	</network>
Alternatively consider to setup network and DNS by other means, as long as the IPs and DNS entries for the bootstrap, masters and workers match.

Second step will be to create a bootstrap VM. This cannot be done with the installer, as the installer expects to have at least one master and a worker. So we can create it manually using libvirt. Consider that you are going to use the previously generated *bootstrap.ign* file, and consume the qcow2 image that has been previously generated. The XML template used to define the bootstrap VM is:

	<domain type='kvm' xmlns:qemu="http://libvirt.org/schemas/domain/qemu/1.0">
	  <name>bootstrap</name>
	  <memory unit='GB'>4</memory>
	  <vcpu placement='static'>2</vcpu>
	  <resource>
	    <partition>/machine</partition>
	  </resource>
	  <os>
	    <type arch='x86_64' machine='pc'>hvm</type>
	    <boot dev='hd'/>
	    <boot dev='network' />
	  </os>
	  <features>
	    <acpi/>
	    <apic/>
	    <pae/>
	  </features>
	  <clock offset='utc'/>
	  <on_poweroff>destroy</on_poweroff>
	  <on_reboot>restart</on_reboot>
	  <on_crash>destroy</on_crash>
	  <devices>
	    <emulator>/usr/libexec/qemu-kvm</emulator>
	    <disk type='file' device='disk'>
	      <driver name='qemu' type='qcow2'/>
	      <source file='/path/to/rhcos-qemu.qcow2'/>
	      <target dev='hda' bus='ide'/>
	      <address type='drive' controller='0' bus='0' target='0' unit='0'/>
	    </disk>
	    <interface type='network'>
	      <mac address='52:54:00:cc:cc:01'/>
	      <source network="cluster" bridge='br0'/>
	      <model type='virtio'/>
	      <target dev='vnet1' />
	      <alias name='net0' />
	      <address type='pci' domain='0x0000' bus='0x00' slot='0x03' function='0x0'/>
	    </interface>
	    <serial type='pty'>
	      <source path='/dev/pts/3'/>
	      <alias name='serial0'/>
	    </serial>
	    <console type='pty' tty='/dev/pts/3'>
	      <source path='/dev/pts/3'/>
	      <target type='serial' port='0'/>
	      <alias name='serial0'/>
	    </console>
	    <graphics type='vnc' port='5901' listen='0.0.0.0' />
	  </devices>
	  <qemu:commandline>
	    <qemu:arg value="-fw_cfg"/>
	    <qemu:arg value="name=opt/com.coreos/config,file=/path/to/ignition/bootstrap.ign"/>
	  </qemu:commandline>
	</domain>

**5. Setup dnmasq for pxe boot**
When using real baremetal servers and not libvirt, the network configured by libvirt may not be enough and ips need to be offered by other means such as DHCP. Also PXE boot needs to be offered. This can be achieved using dnsmasq configuration. Install dnsmasq on your buildhost (or bootstrap vm if connectivity is properly configured to communicate with PXE network), and run dnsmasq on it with the following settings:

    interface=nic_used_for_pxe
    dhcp-range=pxe_range_start,pxe_range_end,lease_time
    dhcp-match=set:ipxe,175 # iPXE sends a 175 option.
    dhcp-boot=tag:!ipxe,undionly.kpxe,<ip_for_tftp>
    dhcp-boot=http://<ip_for_nginx>/bootstrap.ipxe
    dhcp-no-override
    enable-tftp
    tftp-root=/var/lib/tftpboot
   Add any other setting you may need.

   dnsmasq comes with a tftp service. So go into /var/lib/tftpboot, and place undionly.kpxe there. You can get it from */usr/share/ipxe/undionly.kpxe* or download it from [http://boot.ipxe.org/undionly.kpxe](http://boot.ipxe.org/undionly.kpxe)

   The *bootstrap.ipxe* file needs to be served over http. An easy way to setup it, is just to install an nginx server, and place bootstrap.ipxe in the docroot for that nginx. A sample bootstrap.ipxe will look like:

	#!ipxe
	#
	kernel http://<nginx_url>/vmlinuz coreos.inst.install_dev=sda coreos.inst.image_url=http://<nginx_url>/rhcos-qemu.raw.gz coreos.inst=yes rd.neednet=1 coreos.inst.ignition_url=http://<nginx_url>/[master.ign|worker.ign] coreos.inst.ignition_on_kernel_params=1 rd.break=mount
initrd http://<nginx_path>/initrd.img
boot


In order to make it work, you need to previously have copied the ignition files and images inside that nginx doc root.

**5.1. Optionally, enable introspection**
The RCHOS image have an introspection functionality attached. When an
introspection endpoint is provided, the image will generate a JSON blob with all
the information for the machine, based on this tool:
[https://github.com/jaypipes/ghw](https://github.com/jaypipes/ghw)
The introspection endpoint could analyze that, and return an ignition file as a
result of that introspection. This means that the coreos.inst.ignition_url
endpoint wil be ignored, and the ignition file provided to the machine will be
the one returned by the introspection endpoint.
In order to enable introspection, please add this to the kernel parameters in
bootstrap.ipxe:

    coreos.inst.introspection_endpoint=http://<ip_for_introspection_endpoint>

And remove the coreos.inst.ignition_url parameter

**6. Start bootstrap vm and servers**
First start the bootstrap vm and check that it's up and running. After you can see the bootstrap started, just power on the servers (manually or using IPMI). The servers should boot from PXE and get and IP and PXE offer from that dnsmasq, properly reading bootstrap.ipxe.
This will cause that coreos installer image is booted on ramdisk, and will download the final coreos raw image, as well as the ignition file. The final image and ignition file will be copied on the specified disk, and the system will be rebooted.
Once the server is rebooted, it will start from disk and will read the ignition file stored on it. This ignition file can be master or worker, depending on your needs. It will cause the servers to communicate with the **Machine Config Server** that is running on the bootstrap vm, and get all the needed artifacts to be configured properly and join the cluster.
