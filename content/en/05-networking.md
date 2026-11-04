# Networking

Networking in depth from a DevOps/SRE point of view: the OSI and TCP/IP models, IP addresses and subnet masks with manual calculation, routing, NAT, TCP and UDP, DNS, HTTP and TLS, load balancers and firewalls, plus diagnostic tools. Each topic: what it is, why it is needed, how it works, examples and typical problems.

## The OSI model and the TCP/IP stack

**Why a model.** Network communication is split into layers. Each layer solves its own task and uses the services of the layer below without knowing its details. The browser doesn't care whether traffic goes over Wi-Fi or fibre, and the Ethernet cable doesn't care what is inside — HTTP or SSH.

For an engineer, the model is first of all a **language for troubleshooting**: "a problem at L3" means routing or IP, "an L7 load balancer" means it understands HTTP. The model comes up in almost every interview, and the right answer is not a list of names but an understanding of what each layer does.

The model is called **OSI** (Open Systems Interconnection), standardised by ISO (standard ISO/IEC 7498-1). Hence the common mix-up "the ISO model".

| # | OSI layer | What it does | Addressing | Data unit | Examples |
|---|---|---|---|---|---|
| 7 | Application | Application protocols | — | data | HTTP, DNS, SSH, SMTP, gRPC (Google's remote function call protocol over HTTP/2, popular between microservices) |
| 6 | Presentation | Data format, encoding, encryption | — | data | TLS (partly), JSON, compression |
| 5 | Session | Establishing and maintaining a session | — | data | TLS sessions, RPC (Remote Procedure Call — calling a function on another server) |
| 4 | Transport | Delivery between **processes**, reliability | **port** | segment (TCP), datagram (UDP) | TCP, UDP, QUIC (a transport over UDP with built-in encryption, HTTP/3 runs on it) |
| 3 | Network | Delivery between **networks**, routing | **IP address** | packet | IP, ICMP (service messages: ping, "host unreachable"), routing |
| 2 | Data Link | Delivery within one network segment | **MAC address** (the hardware address of a network card) | frame | Ethernet, Wi-Fi, ARP, VLAN (see below) |
| 1 | Physical | Transmitting bits as a signal | — | bit | cable, fibre, radio |

In practice the **TCP/IP stack** is used, which has 4 layers: application (OSI 5–7), transport (4), network (Internet, 3) and link (Link, 1–2). OSI layers 5 and 6 are blurred in real protocols, which is why TLS is placed sometimes at layer 6 and sometimes "between 4 and 7".

**Encapsulation** — how data passes through the layers. When a browser sends an HTTP request, each lower layer adds its own header:
```
HTTP request                                  ← L7: "GET /index.html"
[TCP header: port 51234 → 443][HTTP]          ← L4: from which process to which
[IP: 192.168.1.10 → 93.184.216.34][TCP][HTTP] ← L3: from which node to which
[Ethernet: laptop MAC → router MAC][IP][TCP][HTTP]  ← L2: to the next device
```
At the receiver the headers are removed in reverse order. An important detail: **MAC addresses change on every hop of the path** (from you to the router, from the router to the ISP and so on), while **the source and destination IP addresses stay the same** (unless there is NAT).

**Devices by layer:**
- **L2 switch** — forwards frames by MAC address within one network.
- **L3 router** — forwards packets between different networks by IP.
- **An L4 load balancer** (AWS NLB — Network Load Balancer; IPVS — a load balancer built into the Linux kernel, used by kube-proxy) — distributes connections seeing only IPs and ports. Fast, but it doesn't know which URL was requested.
- **An L7 load balancer** (AWS ALB — Application Load Balancer, nginx, Envoy — the proxy service meshes like Istio are built on) — understands HTTP: it can route by domain and path, terminate TLS (decrypt HTTPS on itself and then send plain HTTP or a new TLS connection onward), add headers.

### 🧪 Mini task

**Task.** Start a local HTTP server and send it a `curl -v` request. In the verbose output find the lines that belong to different layers: the TCP connection setup (L4), the HTTP request and the HTTP response (L7).

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
python3 -m http.server 8770 --bind 127.0.0.1 > /dev/null 2>&1 & srv=$!; sleep 1
curl -sv http://127.0.0.1:8770/ -o /dev/null 2>&1 | grep -E '^\* +Trying|^\* Connected|^> GET|^< HTTP'
kill $srv
```

Expected output:

```text
*   Trying 127.0.0.1:8770...
* Connected to 127.0.0.1 (127.0.0.1) port 8770
> GET / HTTP/1.1
< HTTP/1.0 200 OK
```

Walkthrough: `Trying` and `Connected` are the transport layer: curl opened a TCP connection (the three-way handshake — an exchange of SYN, SYN-ACK, ACK packets, detailed in the TCP section) to the IP and port, DNS isn't needed here. Lines with `>` are what went out at the application layer (method, path, HTTP version), lines with `<` are the server's response. The server answered with HTTP/1.0 — a quirk of the simple Python server. Lower layers (IP, Ethernet) aren't shown by curl; you see them in `tcpdump`. In old curl versions the `Connected` line ends with `(#0)`.

</details>

## IP addresses and subnet masks

### What an IPv4 address is
A 32-bit number which, for convenience, is written as 4 octets (bytes) in decimal: `192.168.10.25`. Each octet is from 0 to 255 (8 bits).
```
192      .168      .10       .25
11000000 .10101000 .00001010 .00011001
```

The address consists of two parts: the **network number** and the **host number within that network**. Where the boundary lies is defined by the **subnet mask**.

### Mask and CIDR
A mask is 32 bits with ones in the "network" part and zeros in the host part. The **CIDR** notation `/24` means "the first 24 bits are the network".
```
IP:     192.168.10.25   = 11000000.10101000.00001010.00011001
mask:   255.255.255.0   = 11111111.11111111.11111111.00000000   (/24)
network: 192.168.10.0   = IP AND mask (bitwise AND)
```
This is exactly how a host decides whether the destination is in the same network: it applies the mask to its own address and to the destination address. If they match, the destination is in the same network and the packet is sent directly. If not, the packet goes to the default gateway.

### How many addresses are in a subnet
Number of addresses = 2^(32 − prefix). In every subnet two addresses are reserved:
- **network address** — all host bits are 0 (`192.168.10.0`);
- **broadcast** — all host bits are 1 (`192.168.10.255`).

So hosts = 2^(32 − prefix) − 2.

| CIDR | Mask | Addresses | Hosts | Where you see it |
|---|---|---|---|---|
| /32 | 255.255.255.255 | 1 | 1 | one specific host (in SG rules, routes) |
| /31 | 255.255.255.254 | 2 | 2 | point-to-point links (a direct connection of exactly two devices, for example two routers; a special case without network/broadcast) |
| /30 | 255.255.255.252 | 4 | 2 | point-to-point links (the old way) |
| /28 | 255.255.255.240 | 16 | 14 | the smallest subnet in AWS |
| /27 | 255.255.255.224 | 32 | 30 | |
| /26 | 255.255.255.192 | 64 | 62 | |
| /24 | 255.255.255.0 | 256 | 254 | the "classic" small network |
| /20 | 255.255.240.0 | 4 096 | 4 094 | a subnet for EKS (Kubernetes in AWS: every pod gets its own IP from the subnet and quickly eats addresses) |
| /16 | 255.255.0.0 | 65 536 | 65 534 | the maximum VPC CIDR (Virtual Private Cloud — your isolated network in AWS) |
| /8 | 255.0.0.0 | 16 777 216 | | the whole 10.0.0.0/8 |

**AWS specifics:** every VPC subnet reserves not 2 but **5** addresses: the network address, `.1` (VPC router), `.2` (DNS), `.3` (reserved for future use) and the last address. A `/24` gives 251 usable addresses, a `/28` gives 11.

### Calculating a subnet by hand
Task: address `10.20.37.5/20`. Find the network address, broadcast and host range.

1. Prefix /20 — the boundary is in the **third octet** (16 bits — the first two octets fully, plus 4 bits of the third).
2. Mask of the third octet: 4 ones and 4 zeros = `11110000` = 240. The full mask: `255.255.240.0`.
3. Block size in the third octet: 256 − 240 = **16**. Networks go in steps of 16: 0, 16, 32, 48...
4. 37 falls into the 32–47 block. So the network starts at `10.20.32.0`.
5. Broadcast is the last address of the block: `10.20.47.255`.
6. Hosts: `10.20.32.1` – `10.20.47.254`, 4094 in total.

The "block size" method works for any mask: 256 minus the mask value in the "interesting" octet.

Another example: `192.168.1.200/26`. Mask of the last octet: 26 − 24 = 2 bits → `11000000` = 192, block = 256 − 192 = 64. Blocks: 0, 64, 128, 192. 200 falls into 192–255. Network `192.168.1.192`, broadcast `192.168.1.255`, hosts `.193`–`.254`.

### Splitting a network into subnets
Task: split `10.0.0.0/16` into 4 equal subnets. 4 parts need 2 bits (2² = 4), so the prefix is /16 + 2 = **/18**. Each one has 2^(32−18) = 16 384 addresses, i.e. a block of 64 in the third octet:
```
10.0.0.0/18     10.0.0.0   – 10.0.63.255
10.0.64.0/18    10.0.64.0  – 10.0.127.255
10.0.128.0/18   10.0.128.0 – 10.0.191.255
10.0.192.0/18   10.0.192.0 – 10.0.255.255
```
**Aggregation (summarisation)** is the reverse operation: `10.0.0.0/24` and `10.0.1.0/24` together give `10.0.0.0/23`. It is used to reduce the number of routes.

In Terraform this is done by the `cidrsubnet` function: `cidrsubnet("10.0.0.0/16", 2, 1)` → `10.0.64.0/18` (add 2 bits to the prefix and take subnet number 1).

### Private and special ranges
Private addresses (RFC 1918) are not routed on the internet; they are used inside organisations and VPCs:
- `10.0.0.0/8` — the largest, usually for VPCs and corporate networks;
- `172.16.0.0/12` (172.16.0.0 – 172.31.255.255) — this includes `172.17.0.0/16`, Docker's default network;
- `192.168.0.0/16` — home networks.

Special ones:
- `127.0.0.0/8` — loopback, the host itself (`127.0.0.1` is localhost);
- `169.254.0.0/16` — link-local (addresses that work only within one network segment and aren't routed; the OS assigns them to itself if there's no DHCP). In AWS, `169.254.169.254` hosts the instance **metadata service** (IMDS, Instance Metadata Service): EC2 gets its temporary IAM role keys from here, which is why access to it from pods in EKS is restricted — otherwise any pod gets the node's permissions;
- `100.64.0.0/10` — CGNAT (Carrier-Grade NAT: a range providers use for NAT of their subscribers; it hardly appears in ordinary networks, so it doesn't conflict with the office). In EKS it's often used as an extra CIDR for pods when the main VPC runs out of addresses;
- `0.0.0.0/0` — "any address", the default route.

### Planning addresses in a VPC
- The CIDRs of different VPCs and the office network **must not overlap**, otherwise you can't set up peering (a direct connection of two VPCs), Transit Gateway (the central AWS router many VPCs and VPNs attach to) and VPN (an encrypted tunnel from the office into the VPC): a router won't know which network to send a packet to. Example: two teams both have VPC `10.0.0.0/16` — a year later they need to be connected, and that's impossible without one of them migrating.
- Take it with headroom: a VPC CIDR can't be changed later (you can only add a secondary one).
- In EKS with the AWS VPC CNI (the network plugin that gives pods real IPs from the VPC subnets) **every pod gets a real IP from the subnet**, so node subnets need to be large (/19–/20), or prefix delegation is used (the node gets a whole /28 block of 16 addresses at once instead of one at a time) plus a separate CIDR for pods.

### IPv6 in a nutshell
128-bit addresses (`2001:db8::1`), practically unlimited in number. A subnet is usually `/64`. NAT in the classic sense is not needed: every device has a global address and protection is provided by the firewall. `::1` is loopback.

### 🧪 Mini task

**Task.** For the `10.0.0.0/22` network compute the mask, the number of addresses, the broadcast, the first and last host address. Check whether `10.0.3.200` and `10.0.4.1` belong to it, split it into `/24`s, and check whether `172.31.5.4` and `172.32.0.1` are private. Work out the answers in your head first, then check with Python.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import ipaddress as ip
net = ip.ip_network("10.0.0.0/22")
print(net.netmask, net.num_addresses, net.broadcast_address)
hosts = list(net.hosts())
print(hosts[0], hosts[-1])
print(ip.ip_address("10.0.3.200") in net, ip.ip_address("10.0.4.1") in net)
print([str(s) for s in net.subnets(new_prefix=24)])
print(ip.ip_address("172.31.5.4").is_private, ip.ip_address("172.32.0.1").is_private)
EOF
```

Expected output:

```text
255.255.252.0 1024 10.0.3.255
10.0.0.1 10.0.3.254
True False
['10.0.0.0/24', '10.0.1.0/24', '10.0.2.0/24', '10.0.3.0/24']
True False
```

Breakdown: a `/22` leaves 32 − 22 = 10 host bits, that's 2¹⁰ = 1024 addresses, and the mask in the third octet is 252 (256 − 4). The network covers the third octet from 0 to 3, so `10.0.3.200` is inside and `10.0.4.1` is already in the next network. One `/22` is four `/24`s. The private range `172.16.0.0/12` ends at `172.31.255.255`, so `172.32.0.1` is public — a common trick question. AWS reserves 5 more addresses in every subnet, leaving 1019 for hosts.

</details>

## How a packet reaches a neighbour: ARP and switching

Within one network (L2), frames are delivered by MAC address. But the application only knows the destination IP. **ARP** is used to find its MAC:
1. The host sends a **broadcast** request to everyone in the network: "Who has IP 192.168.1.20? Tell me your MAC."
2. The host with that IP replies directly: "192.168.1.20 is `aa:bb:cc:dd:ee:ff`".
3. The reply is cached in the ARP table.

```bash
ip neigh            # ARP table (neighbours): IP → MAC
```
If the destination is in **another** network, the host sends the ARP request not for it but for the **gateway** (router) and sends the frame to the gateway.

**VLAN** (Virtual LAN) — a way to split one physical switch into several isolated logical networks: for example, in an office the accounting network, guest Wi-Fi and servers share the same hardware but can't see each other. Frames don't pass between ports of different VLANs without a router. Between switches VLANs are carried over a trunk port (a port carrying frames of many VLANs at once) with an 802.1Q tag (the VLAN number written into the frame).

### 🧪 Mini task

**Task.** Host `192.168.1.10/24` with gateway `192.168.1.1` sends packets to `192.168.1.20`, `192.168.1.254` and `8.8.8.8`. For each address decide whose MAC the host will look up via ARP: the destination's itself or the gateway's.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import ipaddress as ip
me = ip.ip_interface("192.168.1.10/24")
gateway = ip.ip_address("192.168.1.1")
for dst in ["192.168.1.20", "192.168.1.254", "8.8.8.8"]:
    d = ip.ip_address(dst)
    if d in me.network:
        print(f"{dst}: same subnet, ARP for {dst} and send directly")
    else:
        print(f"{dst}: other network, ARP for gateway {gateway}")
EOF
```

Expected output:

```text
192.168.1.20: same subnet, ARP for 192.168.1.20 and send directly
192.168.1.254: same subnet, ARP for 192.168.1.254 and send directly
8.8.8.8: other network, ARP for gateway 192.168.1.1
```

Breakdown: the host applies its mask to the destination address. If the network matches, the frame goes straight to the destination and ARP looks up its MAC. If not, the frame carries the gateway's MAC while the IP destination stays `8.8.8.8`: the MAC changes at every hop, the IP doesn't (without NAT). That's why a wrong mask breaks connectivity "strangely": some addresses work, others don't. To see the real ARP table: `arp -an` on macOS, `ip neigh` on Linux.

</details>

## Routing

A router (and any host — it has a routing table too) decides for every packet where to send it next.

```bash
ip route
# default via 10.0.1.1 dev eth0              ← default route: everything unknown goes to gateway 10.0.1.1
# 10.0.1.0/24 dev eth0 proto kernel src 10.0.1.25   ← own network: send directly via eth0
# 172.17.0.0/16 dev docker0                   ← Docker network

ip route get 8.8.8.8
# 8.8.8.8 via 10.0.1.1 dev eth0 src 10.0.1.25 ← which route will actually be chosen for the address
```

**The selection rule is longest prefix match.** Of all the routes that match an address, the most **specific** one (with the longest prefix) wins. For address `10.0.5.7` with routes `0.0.0.0/0`, `10.0.0.0/16` and `10.0.5.0/24`, the `/24` wins.

This is how route tables work in AWS: a private subnet (a subnet without direct internet access) has a route `10.0.0.0/16 → local` (inside the VPC) and `0.0.0.0/0 → NAT Gateway` (a managed AWS service through which private instances reach the internet under its public IP). Traffic to a neighbouring instance goes via the more specific `local`, and to the internet via NAT.

**Static and dynamic routing.** Static — routes are written by hand (as in a VPC route table). Dynamic — routers exchange routes themselves via protocols: **OSPF** inside an organisation, **BGP** (Border Gateway Protocol) between autonomous systems (an AS is the network of one provider or company with its own number; the internet is thousands of ASes telling each other over BGP which addresses are reachable through them). The whole internet runs on BGP; in AWS it's used in Direct Connect (a dedicated physical link from a company's data centre to AWS) and VPN; in Kubernetes Calico and Cilium announce pod networks to physical routers over BGP.

**traceroute and mtr** show the packet's path: which routers it passes and where it gets lost.
```bash
traceroute -n example.com     # -n — don't resolve names (faster)
mtr -rwc 100 example.com      # traceroute + ping: loss and latency on each hop over 100 packets
```
How it works: every IP packet has a **TTL** (time to live), which is decremented by 1 at each router. When TTL reaches 0, the router drops the packet and sends back ICMP "Time Exceeded". traceroute sends packets with TTL 1, 2, 3... and learns the address of each router on the path from the replies.

### 🧪 Mini task

**Task.** Given a VPC route table: `0.0.0.0/0 → igw` (Internet Gateway — the VPC's exit to the internet), `10.0.0.0/16 → local` (inside the VPC), `10.0.5.0/24 → tgw` (Transit Gateway — to another VPC), `10.0.5.128/25 → vpn` (to the office via VPN). Determine the route for `10.0.5.200`, `10.0.5.10`, `10.0.9.9` and `1.1.1.1` by the longest-prefix rule.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import ipaddress as ip
routes = {"0.0.0.0/0": "igw", "10.0.0.0/16": "local", "10.0.5.0/24": "tgw", "10.0.5.128/25": "vpn"}
table = sorted(((ip.ip_network(n), t) for n, t in routes.items()), key=lambda r: r[0].prefixlen, reverse=True)
for dst in ["10.0.5.200", "10.0.5.10", "10.0.9.9", "1.1.1.1"]:
    net, target = next(r for r in table if ip.ip_address(dst) in r[0])
    print(f"{dst:<11} -> {net} via {target}")
EOF
```

Expected output:

```text
10.0.5.200  -> 10.0.5.128/25 via vpn
10.0.5.10   -> 10.0.5.0/24 via tgw
10.0.9.9    -> 10.0.0.0/16 via local
1.1.1.1     -> 0.0.0.0/0 via igw
```

Breakdown: an address can match several routes; the most specific (longest prefix) wins, not the first in the list. `10.0.5.200` falls into all three private networks, but `/25` is the longest. `10.0.5.10` isn't in the upper half of the `/24`, so it goes to `tgw`. The `/0` default route matches everything and only applies when nothing more specific exists. Linux (`ip route get 1.1.1.1`) and AWS route tables both work this way.

</details>

## NAT — network address translation

**The problem.** Private addresses (10.x, 192.168.x) are not routed on the internet. How does an instance in a private subnet download packages?

**NAT (Network Address Translation)** — the router rewrites addresses in packets and remembers the mapping in a translation table so that it can return the reply to the right sender.
- **SNAT** (source NAT) — the **source** address is rewritten. Many private addresses go out to the internet under one public address. This is how a home router and the **AWS NAT Gateway** work. Different internal hosts are distinguished by source port (PAT, port address translation).
- **DNAT** (destination NAT) — the **destination** address is replaced. Port forwarding: packets to the public `1.2.3.4:80` go to the internal `10.0.1.5:8080`. This is how `docker run -p` works, and **kube-proxy** (the Kubernetes component on every node) for a Service: a packet to the service's virtual IP is rewritten to the IP of one of the pods.

Consequence: through SNAT you can connect **from the inside out**, but nobody outside can initiate a connection in. That is why instances in private subnets are not reachable from the internet but can still download updates.

### 🧪 Mini task

**Task.** Compare your Mac's address on the local network with the address the internet sees you as, and check which one is private. That is your home router's NAT (in AWS a NAT Gateway does the same).

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
local=$(ipconfig getifaddr en0)
public=$(curl -s https://checkip.amazonaws.com)
python3 -c "
import ipaddress, sys
for name, a in (('local', sys.argv[1]), ('public', sys.argv[2])):
    print(name, a, 'private' if ipaddress.ip_address(a).is_private else 'public')
" "$local" "$public"
```

Expected output:

```text
local 192.168.1.23 private
public 203.0.113.45 public
```

Breakdown: your addresses will differ; what matters is the outcome: inside the network a private address, outside the router's public address. The router rewrites the source address (SNAT) and remembers the connection in a translation table to route the reply back to the right device. That's why nobody outside can connect to your Mac first — that needs port forwarding (DNAT). In AWS private subnets reach the internet through a NAT Gateway, and inbound connections to them are impossible. On Linux use `hostname -I` instead of `ipconfig getifaddr en0`.

</details>

## MTU and fragmentation
**MTU** (Maximum Transmission Unit) — the maximum packet size on a link. Ethernet is 1500 bytes. Inside an AWS VPC jumbo frames (frames bigger than the standard 1500 bytes: fewer headers for the same amount of data) of 9001 bytes are supported, but over the internet, VPN and peering it's 1500.

If a packet is bigger than the MTU of a link on the path, it must be fragmented (cut into pieces) or dropped. Modern systems find the allowed size via **Path MTU Discovery** (they send packets with fragmentation forbidden and shrink the size if a router replies "too big"), and that requires the ICMP "Fragmentation Needed" messages. A typical problem: in a VPN or tunnel (VXLAN — an overlay network that wraps frames in UDP, used by Kubernetes networks; IPsec — an encrypted VPN tunnel; both add their own headers and reduce the usable MTU) someone blocks ICMP. Symptom: **small requests get through, but large responses hang** (SSH connects, but `ls` in a big directory hangs; the HTTPS handshake doesn't complete).

### 🧪 Mini task

**Task.** Look at the MTU of the `en0` and `lo0` interfaces and compute, for MTU 1500, 1450 (a VXLAN overlay in Kubernetes) and 9001 (jumbo frames in AWS), the maximum ICMP payload for `ping` with fragmentation forbidden and the TCP MSS (Maximum Segment Size — how many bytes of payload fit into one TCP segment; the sides tell each other during the handshake).

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
ifconfig en0 | grep -o 'mtu [0-9]*'
ifconfig lo0 | grep -o 'mtu [0-9]*'
for mtu in 1500 1450 9001; do
  echo "mtu=$mtu ping_payload=$(( mtu - 20 - 8 )) tcp_mss=$(( mtu - 20 - 20 ))"
done
```

Expected output:

```text
mtu 1500
mtu 16384
mtu=1500 ping_payload=1472 tcp_mss=1460
mtu=1450 ping_payload=1422 tcp_mss=1410
mtu=9001 ping_payload=8973 tcp_mss=8961
```

Breakdown: the MTU is the largest IP packet on a link. An ICMP packet is a 20-byte IP header + 8 bytes of ICMP + the payload, so `ping -D -s 1472` (Linux: `ping -M do -s 1472`) is the biggest packet that crosses an MTU-1500 network without fragmenting, and 1473 returns an error. The MSS is how much data TCP puts in a segment: minus 20 bytes of IP and 20 of TCP. An overlay eats 50 bytes for encapsulation, and if the MTU isn't reduced large packets "hang" — the classic symptom: ping works, but TLS or a large response doesn't get through.

</details>

## TCP and UDP

Both protocols are the transport layer (L4). Their job is to deliver data not just to a computer but to a **specific process**. **Ports** (numbers 0–65535) are used for this. A connection is uniquely identified by a 4-tuple: source IP and port, destination IP and port.

### TCP — reliable stream delivery
TCP guarantees that data arrives **completely**, **uncorrupted** and **in the right order**. How:
- every byte is assigned a **sequence number**;
- the receiver acknowledges receipt (**ACK**);
- unacknowledged data is sent again (**retransmission**);
- **flow control** (the receive window): the sender doesn't send more than the receiver can process;
- **congestion control**: the rate drops when the network loses packets.

The price: connection setup, overhead, latency on loss (one lost packet delays all the following ones — head-of-line blocking).

### Connection setup: the three-way handshake
```
Client                          Server
  |  ------ SYN (seq=x) ------>    |   "I want to connect, my number is x"
  |  <--- SYN-ACK (seq=y, ack=x+1) |   "Agreed, my number is y, got your x"
  |  ------ ACK (ack=y+1) ----->   |   "Got your y". The connection is established
```
Why three steps: both sides have to agree on each other's initial sequence numbers and make sure the link works in both directions.

Closing takes four steps: each side sends a **FIN** and receives an **ACK** for it (each side closes its half of the connection independently). **RST** is an abrupt reset of the connection, for example when connecting to a closed port.

### TCP connection states
```bash
ss -tan | awk 'NR>1 {print $1}' | sort | uniq -c    # how many connections are in each state
```
- **LISTEN** — the server is waiting for connections on a port.
- **ESTABLISHED** — the connection is established, data is being exchanged.
- **TIME_WAIT** — on the side that **closed** the connection **first**. It's held for 2×MSL (Maximum Segment Lifetime — how long a packet may wander the network; in Linux the result is 60 seconds) so that late packets of the old connection don't end up in a new one with the same ports. Many TIME_WAITs appear on clients that open a new connection for every request: on a busy service there are tens of thousands, and ephemeral ports can run out. The cure is connection reuse (keep-alive — keeping the connection open for the next requests, connection pools).
- **CLOSE_WAIT** — the other side has closed the connection, but **our application hasn't closed the socket yet**. Accumulating CLOSE_WAIT is almost always an **application bug** (connections are not closed) that ends with running out of file descriptors.
- **SYN_SENT** — the client sent a SYN and is waiting for a reply. If it hangs, packets are being dropped on the way (firewall, Security Group).

The difference between "connection refused" and "timeout" is an important diagnostic sign:
- **Connection refused** — the packet reached the host, but nobody is listening on the port and the host replied with RST. The network is fine; the problem is the service.
- **Connection timed out** — no response at all. Packets are dropped somewhere: a firewall, a Security Group, a NACL (Network ACL — a subnet-level firewall in AWS), no route, the host is off.

### TCP and latency: what adds milliseconds
When a latency SLO is measured in single milliseconds, TCP details that are usually invisible start to matter.

- **Nagle's algorithm and delayed ACK.** Nagle holds back a small packet until the previous one is acknowledged. Delayed ACK on the other side waits up to ~40 ms to acknowledge together with the response. Together they produce the classic ~40 ms delay on small requests. The fix is the **`TCP_NODELAY`** socket option (it disables Nagle), set by the application; most HTTP and RPC libraries do it themselves.
- **A new connection is a handshake (1 RTT) plus TLS (another 1 RTT in TLS 1.3).** RTT (round-trip time) is the "there and back" time between two hosts. For low latency, connections are kept open and reused (keep-alive, pools, HTTP/2 and gRPC with multiplexing — many requests in parallel over one connection).
- **A lost packet is expensive.** The minimum retransmission timeout (RTO) in Linux is 200 ms. One lost packet that wasn't recovered by fast retransmit (resending right away when the receiver has acknowledged the same thing three times, without waiting for the timeout) adds +200 ms to a request — 40 times a 5 ms budget. That's why such systems watch losses closely: `nstat` (TcpRetransSegs), `ss -ti` (the `retrans` field), `tcpretrans` from bcc.
- **Buffers and windows.** For fast links the buffer size must cover the product of bandwidth and RTT (the bandwidth-delay product — how much data is "in flight" at once: 10 Gbit/s × 10 ms = 12.5 MB), otherwise the flow hits the window and can't speed up. Too-large buffers, on the other hand, accumulate a queue and increase latency (bufferbloat — packets sit in queues for a long time instead of being dropped and signalling to slow down).
- **Connection idle.** `net.ipv4.tcp_slow_start_after_idle=1` (the default) restarts slow start after a pause (slow start — TCP begins with a small window and grows it, checking that the network copes) — the first packets after silence go slower. For long-lived connections with pauses it's turned off (`=0`).
- **RTT between hosts.** Within one AZ it is usually hundreds of microseconds, between AZs — around a millisecond. With a 5 ms budget, a cross-zone call in the hot path is a noticeable share. A cluster placement group reduces RTT even further.

```bash
ss -tin dst 10.0.1.20     # rtt:0.21/0.05 — mean RTT and its variation in ms, rto, cwnd, retrans for each connection
nstat -az TcpRetransSegs TcpExtTCPTimeouts
```

### UDP — fast and without guarantees
UDP simply sends datagrams: no connection, acknowledgements, retransmissions or ordering. Why such a protocol:
- **DNS** — one small request, one response; if it is lost, the client asks again. Setting up a TCP connection would take longer than the request itself.
- **Streaming video and voice** — a late packet is no longer needed; it's better to skip it than to wait.
- **NTP** (time synchronisation), **syslog** (classic log shipping), **StatsD metrics** (a "fire and forget" metrics protocol: losing one data point doesn't matter, but the application never waits).
- **QUIC (HTTP/3)** — implements reliability itself on top of UDP to avoid TCP's drawbacks.

### Ports you should know
| Port | Service |
|---|---|
| 22 | SSH |
| 53 | DNS (UDP and TCP) |
| 80 / 443 | HTTP / HTTPS |
| 25, 587 | SMTP |
| 123 | NTP (UDP) |
| 3306 | MySQL |
| 5432 | PostgreSQL |
| 6379 | Redis |
| 9092 | Kafka |
| 6443 | Kubernetes API server |
| 2379–2380 | etcd |
| 10250 | kubelet |
| 9090 / 9100 | Prometheus / node_exporter |
| 30000–32767 | Kubernetes NodePort |

Ports below 1024 are "privileged": only root (or a process with the `CAP_NET_BIND_SERVICE` capability) can listen on them. Outgoing connections use **ephemeral ports** (32768–60999 in Linux, see `sysctl net.ipv4.ip_local_port_range`). This matters for NACLs in AWS: they're stateless (they don't remember connections, every packet is checked separately — unlike a Security Group, which lets replies through by itself), and return traffic to ephemeral ports must be allowed explicitly. A classic mistake: the NACL allows inbound 443 but doesn't allow outbound replies to clients on ports 32768–60999 — the site "hangs".

### 🧪 Mini task

**Task.** Use Python sockets to see the difference between the protocols: connect over TCP to a closed port, send a UDP datagram to the same port, then start a TCP server, establish a connection, look at the client's port and send some data.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import socket
try:
    socket.create_connection(("127.0.0.1", 8799), timeout=2)
except ConnectionRefusedError as e:
    print("tcp to closed port:", e.strerror)
u = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
print("udp to closed port, bytes sent:", u.sendto(b"ping", ("127.0.0.1", 8799)))
srv = socket.socket()
srv.bind(("127.0.0.1", 0))
srv.listen()
port = srv.getsockname()[1]
client = socket.create_connection(("127.0.0.1", port))
conn, addr = srv.accept()
print("handshake done, client port is ephemeral:", addr[1] >= 1024)
conn.sendall(b"hello")
print("tcp received:", client.recv(5))
EOF
```

Expected output:

```text
tcp to closed port: Connection refused
udp to closed port, bytes sent: 4
handshake done, client port is ephemeral: True
tcp received: b'hello'
```

Breakdown: TCP sets up a connection before sending data. On a closed port the kernel immediately answers with RST, hence the instant `Connection refused`. If a firewall silently dropped the packets you'd get a timeout instead — that difference is how you diagnose the problem. UDP doesn't set up connections: the datagram "successfully" left although nobody is waiting for it, and the sender never learns that. The OS gave the client a random ephemeral port — the (client IP:port, server IP:port) pair is what identifies the connection.

</details>

## DNS

**Why.** People and configs use names (`api.example.com`), while the network works with IP addresses. DNS is a distributed "name → address" database (and more).

### How name resolution works
Suppose an application wants the address of `api.example.com`:
1. The local cache and the `/etc/hosts` file are checked.
2. The query goes to a **recursive resolver** (its address is in `/etc/resolv.conf`: the ISP's resolver, `8.8.8.8`, in AWS — `169.254.169.253` or `.2` in the VPC). It does all the work on behalf of the client.
3. The resolver asks a **root server**: "Who is responsible for `.com`?" It gets the list of `.com` zone servers.
4. It asks a `.com` server: "Who is responsible for `example.com`?" It gets the domain's **authoritative** servers (for example, Route 53).
5. It asks the authoritative server: "What is the address of `api.example.com`?" It gets the answer.
6. The resolver **caches** the answer for the **TTL** of the record and returns it to the client.

```bash
dig api.example.com                 # full answer: records, TTL, which server answered
dig +short api.example.com          # addresses only
dig @8.8.8.8 api.example.com        # ask a specific resolver (compare with the local one)
dig +trace api.example.com          # walk the whole chain from the root — where does it break?
dig api.example.com NS              # the domain's authoritative servers
dig -x 8.8.8.8                      # reverse lookup: IP → name (PTR)
cat /etc/resolv.conf                # which resolver is used, search domains
getent hosts api.example.com        # resolve "like the application does" (with /etc/hosts and nsswitch)
```

### Record types
| Record | What it stores | Example use |
|---|---|---|
| **A** | name → IPv4 | `api.example.com → 1.2.3.4` |
| **AAAA** | name → IPv6 | |
| **CNAME** | name → another name (alias) | `www.example.com → example-lb-123.elb.amazonaws.com` |
| **MX** | the domain's mail servers (with priority) | |
| **TXT** | arbitrary text | SPF (which servers may send mail on behalf of the domain), DKIM (the public key for verifying email signatures), domain ownership verification (ACM — the AWS certificate service, Google) |
| **NS** | the zone's authoritative servers | delegating a subdomain to another DNS provider |
| **SOA** | zone parameters (primary server, serial number, TTL for negative answers) | |
| **PTR** | IP → name (reverse zone) | mail servers, logs |
| **SRV** | service → host and port | Kerberos, LDAP (corporate authentication and user directory), Kubernetes headless services (a service without a virtual IP: DNS returns pod addresses directly) |
| **CAA** | which certificate authorities may issue certificates for the domain | protection against mistaken issuance |

**A CNAME cannot be placed at the root of a domain** (`example.com`, the "apex"): the root must have SOA and NS records, and a CNAME cannot coexist with other records. That is why for a load balancer at the domain root Route 53 uses an **Alias record** (an internal AWS mechanism that returns A records), and other providers offer ALIAS/ANAME or CNAME flattening (Cloudflare).

### TTL and why "DNS doesn't update immediately"
TTL is how many seconds resolvers may cache an answer. If a record has a TTL of 86400 (a day), clients may keep going to the old address for a day after the IP changes. Practice for migrations: **in advance** (one TTL before the switch) lower the TTL to 60 seconds, switch, and raise it back once things are stable.

### DNS in Kubernetes
A pod gets an `/etc/resolv.conf` with the address of CoreDNS (the DNS server inside the cluster; it runs as pods in `kube-system` and knows the names of all Services) and search domains (suffixes the resolver tries appending to a short name):
```
nameserver 172.20.0.10
search prod.svc.cluster.local svc.cluster.local cluster.local ec2.internal
options ndots:5
```
- A Service is reachable at `name.namespace.svc.cluster.local`, and within its own namespace simply as `name`.
- **`ndots:5`**: if a name has fewer than 5 dots, the resolver first tries appending each search domain. A query for `api.example.com` (2 dots) produces queries for `api.example.com.prod.svc.cluster.local`, `api.example.com.svc.cluster.local` and so on before asking for the name itself. This multiplies the load on CoreDNS and the latency. Fixes: a trailing dot (`api.example.com.` — a fully qualified name, search isn't applied), lower `ndots` in the pod's `dnsConfig`, NodeLocal DNSCache (a DNS cache running on every node as a DaemonSet: repeated queries don't go over the network to CoreDNS).

### DNS in production: hybrid networks, failures and automation
**Split-horizon DNS** — the same name answers differently from inside and outside: `api.example.com` from the internet leads to a public load balancer, and from the VPC — to an internal one. In Route 53 these are a public and a private hosted zone with the same name; only the associated VPCs see the private one.

**DNS between clouds and a data centre.** By default each network knows only its own names: a VPC doesn't resolve names from Cloud DNS (GCP) or from the corporate AD. The solution is query forwarding:
- a **Route 53 Resolver outbound endpoint** + a rule: "forward queries for `gcp.internal.example.com` to the IPs of the DNS servers in GCP";
- a **Route 53 Resolver inbound endpoint** — IP addresses in the VPC to which other networks forward queries for AWS names (in GCP this is a forwarding zone in Cloud DNS pointing at these IPs);
- DNS traffic goes over the VPN or Interconnect, so the endpoints' Security Groups and the routes must let UDP and TCP 53 through.

**Why queries in Kubernetes sometimes answer after exactly 5 seconds.** That's the resolver's retry timeout. The classic cause is a race in conntrack (the Linux kernel's connection tracking table): glibc sends the A and AAAA queries simultaneously from one UDP socket, and on a collision one of the packets is lost in NAT. The client waits 5 seconds and retries. The fixes are **NodeLocal DNSCache** (a cache on every node, bypasses conntrack), the `single-request-reopen` option in the pod's `dnsConfig`, or reducing the number of queries (`ndots`, a trailing dot in the name). To check: `kubectl exec ... -- time getent hosts api.example.com` in a loop and the CoreDNS error and latency metrics.

**CoreDNS is also a service that needs scaling.** Two replicas by default can't cope with thousands of pods with `ndots:5`. Scaling by the number of nodes and cores — `cluster-proportional-autoscaler`, monitoring — the `coredns_dns_request_duration_seconds` and `coredns_forward_requests_total` metrics, plus an alert on `SERVFAIL`.

**DNS caching on the client side.** Even with a 60-second TTL a switch to the standby region may not reach the applications: the JVM caches answers itself (`networkaddress.cache.ttl`, 30 seconds by default, and forever in old configurations with a security manager), connection pools keep open TCP sessions to the old address for hours, and **negative caching** (caching the "name doesn't exist" answer, with the time from the SOA) can keep an error if a record was deleted and recreated. DR planning takes this into account: a short TTL, a limited connection lifetime in the pools, checking the switch during drills.

**ExternalDNS** is a Kubernetes controller that creates records in Route 53 (Cloud DNS, Cloudflare) for Ingresses and Services by itself, based on an annotation:
```yaml
metadata:
  annotations:
    external-dns.alpha.kubernetes.io/hostname: shop.example.com
    external-dns.alpha.kubernetes.io/ttl: "60"
```
To keep ExternalDNS from deleting other people's records, it has a **TXT registry** (next to each of its records it creates a TXT with an owner label) and `--domain-filter`, which restricts the zones.

### 🧪 Mini task

**Task.** Use `dig +short` to get the A records of `one.one.one.one`, the reverse PTR record for `1.1.1.1` and the chain for `www.github.com`. Then look at the full answer with the TTL and record type.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
dig +short one.one.one.one A | sort
dig +short -x 1.1.1.1
dig +short www.github.com | head -1
dig one.one.one.one A +noall +answer | awk '{print $1, $4, $5}' | sort
```

Expected output:

```text
1.0.0.1
1.1.1.1
one.one.one.one.
github.com.
one.one.one.one. A 1.0.0.1
one.one.one.one. A 1.1.1.1
```

Breakdown: a name can have several A records — clients get them in varying order, the simplest DNS load balancing (hence `sort`). `-x` builds a reverse query in the `in-addr.arpa` zone and finds the PTR. `www.github.com` is a CNAME to `github.com`: `dig +short` first prints the CNAME target, then its addresses. The trailing dot marks a fully qualified name (FQDN). The full answer also has the TTL — how many seconds resolvers may cache the record; it decreases on repeated queries, so it's dropped here.

</details>

## HTTP

HTTP is an application-layer "request — response" protocol over TCP (or QUIC).
```bash
curl -v https://api.example.com/health
# > GET /health HTTP/1.1            ← method, path, version
# > Host: api.example.com           ← request headers (Host is required: many sites share one IP)
# > User-Agent: curl/8.4.0
# < HTTP/1.1 200 OK                 ← response status
# < Content-Type: application/json  ← response headers
# {"status":"ok"}                   ← body
```

**Methods:**
- `GET` — get a resource. `POST` — create or perform an action. `PUT` — replace entirely. `PATCH` — change partially. `DELETE` — delete. `HEAD` — like GET, but headers only.
- **Idempotency**: repeating a request gives the same result as a single one. GET, PUT and DELETE are idempotent, POST is not. That's why load balancers and client libraries safely retry only idempotent requests: retrying a POST can create two orders.

**Status codes:**
- **2xx** — success: 200 OK, 201 Created, 204 No Content.
- **3xx** — redirect: 301 (permanent), 302 (temporary), 304 Not Modified (the cache is up to date).
- **4xx** — client error: 400 Bad Request, **401** Unauthorized (not authenticated — who are you?), **403** Forbidden (authenticated but no permission), 404 Not Found, 405 Method Not Allowed, 429 Too Many Requests (rate limit).
- **5xx** — server error: 500 Internal Server Error (an exception in the application), **502** Bad Gateway, **503** Service Unavailable, **504** Gateway Timeout.

**502, 503 and 504 on a load balancer** are a frequent question:
- **502** — the proxy got an **invalid response** from the backend or the backend dropped the connection: the application crashed mid-request, the port is wrong, the backend closed the keep-alive connection before the load balancer did (the application's keep-alive timeout is shorter than the ALB's — a classic).
- **503** — **no healthy backends**: all of them failed the health check (a periodic request from the load balancer to a backend, for example `GET /health`: if it doesn't answer in time or returns non-200, the backend is taken out of rotation), the target group (the group of backends ALB sends traffic to) is empty, the service is overloaded.
- **504** — the backend **didn't respond in time** within the proxy's timeout: a slow DB query, a hang.

**Versions:**
- **HTTP/1.1** — keep-alive (several requests over one connection, but sequentially).
- **HTTP/2** — multiplexing: many parallel requests over one connection, binary format, header compression. gRPC runs on it.
- **HTTP/3** — over QUIC (UDP): faster connection setup and no blocking on packet loss.

### 🧪 Mini task

**Task.** Start a local server with a `docs` folder and check the response codes with `curl`: a page, the folder without a trailing slash (a redirect), following the redirect, a non-existent path. Use a `HEAD` request to look at the `Content-Type` and `Content-Length` headers.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && mkdir docs && echo ok > docs/index.html
python3 -m http.server 8771 --bind 127.0.0.1 > /dev/null 2>&1 & srv=$!; sleep 1
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8771/docs/
curl -s -o /dev/null -w '%{http_code} -> %{redirect_url}\n' http://127.0.0.1:8771/docs
curl -sL http://127.0.0.1:8771/docs
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8771/missing
curl -sI http://127.0.0.1:8771/docs/index.html | grep -iE '^content-(type|length)'
kill $srv
```

Expected output:

```text
200
301 -> http://127.0.0.1:8771/docs/
ok
404
Content-type: text/html
Content-Length: 3
```

Breakdown: `-w '%{http_code}'` is the handiest way to check the status in scripts and health checks. For the folder without a slash the server answered 301 with a `Location` header, and without `-L` curl doesn't follow redirects — a common cause of "empty" responses in scripts. 404 means "no such resource", a client error, not a server one. `-I` sends `HEAD`: headers only, no body. `Content-Length: 3` is `ok` plus a newline.

</details>

## TLS and HTTPS

TLS provides three things: **encryption** (nobody can read it), **integrity** (nobody can change it unnoticed) and **server authentication** (you are talking to the real `bank.com`, not an impostor).

### How the TLS 1.3 handshake works
1. **ClientHello**: the client sends the versions and ciphers it supports, its part of the key exchange (key share, ECDHE — Diffie-Hellman on elliptic curves, a way to agree on a secret over an open channel) and **SNI** (Server Name Indication) — the name of the server it's connecting to.
2. **ServerHello**: the server picks a cipher and sends its part of the key, its **certificate** and a signature proving possession of the private key.
3. Both sides compute a shared secret key from the exchanged parts (Diffie–Hellman: the key itself is never sent over the network).
4. The client **verifies the certificate**:
   - it is signed by a trusted certificate authority (CA) through a chain of intermediate certificates up to a root from the OS trust store;
   - it hasn't expired;
   - the name it connected to is in the certificate's **SAN** (Subject Alternative Name) field.
5. After that, data is encrypted with a symmetric key. TLS 1.3 needs one request-response round trip (1-RTT); TLS 1.2 needed two.

**SNI** is needed because hundreds of domains are served on one IP address (for example, a load balancer or CDN), and the server has to know which certificate to present before HTTP even starts.

**Common problems:**
- an expired certificate (you need automation: cert-manager — a Kubernetes controller that issues and renews certificates itself, for example from Let's Encrypt; ACM — AWS Certificate Manager, free auto-renewing certificates for ALB and CloudFront; plus expiry monitoring);
- the server doesn't send the intermediate certificate (browsers sometimes cope, curl and Java don't: "unable to get local issuer certificate");
- the name doesn't match the SAN;
- the client has an outdated root certificate store (old Docker images without `ca-certificates`).

```bash
openssl s_client -connect api.example.com:443 -servername api.example.com </dev/null
# shows the certificate chain, TLS version, cipher and the verification result (Verify return code: 0 (ok))
openssl s_client -connect host:443 -servername host </dev/null 2>/dev/null | openssl x509 -noout -dates -subject -ext subjectAltName
curl -vI https://api.example.com      # also shows TLS and certificate details
```

**mTLS (mutual TLS)** — both sides present a certificate, and the server verifies the client too. It's used between Kubernetes components, in a service mesh (Istio, Linkerd — a layer that puts a "sidecar" proxy next to every pod and encrypts all traffic between services without code changes), for access to internal APIs.

### PKI and machine identity
When mTLS is used between your own services and hosts, you need your own **certificate authority (CA)** and a process for issuing certificates to machines. This is what **machine identity** means: every host and service proves who it is with its certificate.

**CA hierarchy:**
- **the root CA** — kept offline or in an HSM (Hardware Security Module — a device from which the private key can't be extracted; it performs the signing itself), used rarely, only to sign the intermediates;
- **intermediate (issuing) CA** — signs host and service certificates. If it is compromised, it is revoked and a new one is issued without touching the root that all clients trust.

**Short-lived certificates instead of revocation.** Revocation (CRL — Certificate Revocation List, a list of revoked certificates; OCSP — an online "is this certificate still valid?" query) works poorly in practice: clients often don't check it. It's simpler to issue certificates for hours or days and **rotate them automatically**. A stolen certificate quickly becomes useless.

**How a machine gets a certificate (bootstrap):** it first has to prove somehow who it is. Options:
- on AWS — via the instance IAM role or the signed instance identity document (JSON about the instance itself, signed by AWS: it can't be forged): Vault (HashiCorp Vault — a secrets store and its own CA; the `aws` auth method) or AWS Private CA issue a certificate only to an instance with the right role;
- **SPIFFE/SPIRE** — a workload identity standard: an agent on the host verifies who it is (by cloud metadata, process, Kubernetes service account) and issues a short-lived certificate with an ID like `spiffe://example.org/runtime/host`;
- in Kubernetes — cert-manager issuing from Vault or your own CA.

**Tools:** HashiCorp Vault (PKI secrets engine), AWS Private CA, smallstep `step-ca`, cert-manager.

**Operations:**
- rotation **without a restart**: the application re-reads the certificate from disk on a signal or when the file changes;
- **expiry monitoring** of all certificates (`probe_ssl_earliest_cert_expiry` from the blackbox exporter — a Prometheus exporter that checks services "from outside": HTTP, TCP, TLS; x509-certificate-exporter — reads certificates from files and Kubernetes secrets) with an alert long before expiry;
- the key is generated **on the machine itself** and never leaves it — only the certificate signing request (CSR) goes to the CA;
- for installations at a customer's site (on-prem) — a separate CA per customer, so that a compromise at one doesn't affect the others.

**TLS termination** — where the traffic is decrypted. Most often on the load balancer (ALB with a certificate from ACM): from there HTTP goes to the pods inside the VPC. If security requirements demand encryption all the way to the application (PCI DSS for payment data, "zero trust") — re-encryption to the backend (ALB opens a new TLS connection to the pod) or TLS passthrough on an NLB (the load balancer doesn't decrypt the traffic at all; TLS terminates in the application itself).

### 🧪 Mini task

**Task.** Make a lab CA and use it to issue a 30-day certificate for `api.local` with SANs `api.local` and `api.internal`. Check the chain, the subject and issuer, the SANs, whether it matches the names `api.internal` and `evil.com`, and whether it expires within 60 days.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -out ca.crt -days 365 -subj "/CN=Lab CA" 2>/dev/null
openssl req -newkey rsa:2048 -nodes -keyout api.key -out api.csr -subj "/CN=api.local" 2>/dev/null
echo 'subjectAltName=DNS:api.local,DNS:api.internal' > san.ext
openssl x509 -req -in api.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 30 -extfile san.ext -out api.crt 2>/dev/null
openssl verify -CAfile ca.crt api.crt
openssl x509 -in api.crt -noout -subject -issuer
openssl x509 -in api.crt -noout -ext subjectAltName
openssl x509 -in api.crt -noout -checkhost api.internal
openssl x509 -in api.crt -noout -checkhost evil.com
openssl x509 -in api.crt -noout -checkend $(( 60 * 86400 )); echo "exit=$?"
```

Expected output:

```text
api.crt: OK
subject=CN=api.local
issuer=CN=Lab CA
X509v3 Subject Alternative Name:
    DNS:api.local, DNS:api.internal
Hostname api.internal does match certificate
Hostname evil.com does NOT match certificate
Certificate will expire
exit=1
```

Breakdown: `verify` checked the certificate's signature with the CA key — that's how a client builds the chain of trust up to a root in its store. The hostname is matched against the SANs, not the CN: browsers ignore the CN these days, so a certificate without SANs doesn't work. `-checkend` returns 1 if the certificate expires within the given number of seconds — a ready-made check for monitoring and cron. For a live server it's the same: `openssl s_client -connect host:443 -servername host | openssl x509 -noout -dates`.

</details>

## Firewalls

### Stateful and stateless
- A **stateful** firewall **tracks connections**: if an outgoing request is allowed, the reply to it is let through automatically. Example — a **Security Group** in AWS, iptables with the conntrack module (connection tracking — the kernel table of all current connections and their state).
- A **stateless** firewall checks **each packet independently**: the reply also has to be allowed by a separate rule. Example — a **Network ACL** in AWS. That is why in a NACL you have to allow return traffic to ephemeral ports 1024–65535, and this is a frequent cause of "I allowed everything and it still doesn't work".

### iptables in a nutshell
A packet passes through **chains**, each containing rules that are checked in order until the first match:
- `INPUT` — packets addressed to this host;
- `OUTPUT` — outgoing from this host;
- `FORWARD` — passing through the host (router, Docker, Kubernetes);
- `PREROUTING` / `POSTROUTING` (the `nat` table) — this is where DNAT and SNAT happen.

```bash
iptables -L -n -v --line-numbers        # filter table rules with packet counters
iptables -t nat -L -n -v                # nat table: Docker and kube-proxy rules
iptables -A INPUT -p tcp --dport 22 -s 10.0.0.0/8 -j ACCEPT   # allow SSH from the internal network
```
The modern replacement is **nftables**. Docker and kube-proxy (iptables mode) create their own chains: a Kubernetes cluster can have thousands of them, every packet is checked against a long list, and every Service change rewrites the whole table. That's why large clusters use IPVS mode (the kernel load balancer with hash tables — lookup doesn't depend on the number of services) or eBPF (Cilium — a network plugin that replaces kube-proxy with eBPF programs in the kernel).

### 🧪 Mini task

**Task.** Model a firewall `INPUT` chain with a `DROP` policy: replies to established connections are allowed, SSH only from `10.0.0.0/8`, and HTTPS from anywhere. Run five packets through it and see which rule matched each one.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import ipaddress as ip
rules = [
    ("ACCEPT established", lambda p: p["state"] == "ESTABLISHED"),
    ("ACCEPT ssh from 10/8", lambda p: p["port"] == 22 and ip.ip_address(p["src"]) in ip.ip_network("10.0.0.0/8")),
    ("ACCEPT https", lambda p: p["port"] == 443),
]
packets = [
    {"src": "10.1.2.3", "port": 22, "state": "NEW"},
    {"src": "203.0.113.9", "port": 22, "state": "NEW"},
    {"src": "203.0.113.9", "port": 443, "state": "NEW"},
    {"src": "8.8.8.8", "port": 51234, "state": "ESTABLISHED"},
    {"src": "8.8.8.8", "port": 51234, "state": "NEW"},
]
for p in packets:
    verdict = next((name for name, match in rules if match(p)), "DROP (policy)")
    print(f"{p['src']:<12} :{p['port']:<5} {p['state']:<11} -> {verdict}")
EOF
```

Expected output:

```text
10.1.2.3     :22    NEW         -> ACCEPT ssh from 10/8
203.0.113.9  :22    NEW         -> DROP (policy)
203.0.113.9  :443   NEW         -> ACCEPT https
8.8.8.8      :51234 ESTABLISHED -> ACCEPT established
8.8.8.8      :51234 NEW         -> DROP (policy)
```

Breakdown: iptables checks rules top to bottom until the first match, and if nothing matches it applies the chain policy. The `ESTABLISHED` rule makes the firewall stateful: the DNS server's reply to your query got in although port 51234 isn't open anywhere, while the same packet without an established connection was dropped. AWS Security Groups are stateful too (replies are let through automatically), whereas Network ACLs are stateless, and return traffic to ephemeral ports must be allowed explicitly. Real rules: `iptables -L INPUT -n -v --line-numbers` or `nft list ruleset`.

</details>

## Load balancing
A load balancer distributes requests across several backends and removes unhealthy ones from rotation (based on health checks).
- **L4** (AWS NLB, IPVS): sees only IPs and ports. Very fast, can preserve the client IP, suits any TCP/UDP protocol (databases, MQTT — a messaging protocol for IoT devices, gRPC without L7 logic).
- **L7** (AWS ALB, nginx, Envoy, Traefik): understands HTTP. Routing by domain and path (`/api` → one service, `/static` → another), TLS termination, header rewriting, WAF (Web Application Firewall — filtering attacks at the HTTP level: SQL injections, bots, rate limiting), sticky sessions by cookie (the client always lands on the same backend — needed if the session is kept in the server's memory).

Algorithms: **round robin**, **least connections** (to where there are fewer active connections — better for requests of varying duration), **hash** by client IP or a key (one client — one backend, for caches).

**Client IP behind a load balancer.** An L7 load balancer opens its own connection to the backend, so the application sees the load balancer's IP. The real client IP is passed in the `X-Forwarded-For` header (for HTTP) or via the **Proxy Protocol** (for L4, NLB: the load balancer sends a short line with the client's original IP and port at the very start of the TCP connection, and the backend, for example nginx, must be able to read it). Why it matters: logs, rate limiting, geo-blocking — they're useless with the load balancer's IP.

### 🧪 Mini task

**Task.** Model round robin over three backends where `c` fails its health check after the third request, and count how many requests each got. Then pick a backend with the least connections algorithm when they have 3, 1 and 2 active connections.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
from collections import Counter
from itertools import cycle
healthy = ["a", "b", "c"]
rr, served = cycle(healthy), Counter()
for request in range(1, 10):
    if request == 4:
        healthy.remove("c")
        rr = cycle(healthy)
    served[next(rr)] += 1
print("round robin:", dict(served))
active = {"a": 3, "b": 1, "c": 2}
print("least connections ->", min(active, key=active.get))
EOF
```

Expected output:

```text
round robin: {'a': 4, 'b': 4, 'c': 1}
least connections -> b
```

Breakdown: round robin hands out requests in a circle, and while `c` was alive it got its share. After the failed health check the balancer removed it and the load split between the rest. Without health checks every third request would go to a dead backend. Least connections looks at current load and helps when requests have very different durations (WebSockets, long downloads): round robin doesn't see that difference.

</details>

## Diagnostic tools
```bash
# addresses, routes, interfaces
ip -br a                         # short list of interfaces and addresses
ip route; ip route get 10.0.5.7  # routing table; which route will be chosen for an address
ip -s link                       # interface statistics: errors, dropped packets

# who is listening and which connections exist
ss -tlpn                         # TCP (-t), listening (-l), with process (-p), no resolving (-n)
ss -ulpn                         # the same for UDP
ss -tan state established '( dport = :5432 )'   # active connections to Postgres

# reachability
ping -c 4 host                   # ICMP: is the host alive (ICMP is often blocked — that doesn't mean the host is dead)
nc -zv host 5432                 # is a TCP port open (-z — check only, -v — verbose)
curl -v telnet://host:5432       # the same via curl if nc is missing
mtr -rwc 50 host                 # path and loss on each hop

# DNS
dig +short host; dig +trace host; getent hosts host

# HTTP and TLS
curl -sv -o /dev/null https://host/health
curl --resolve host:443:10.0.1.5 https://host/   # substitute the IP while keeping the name for SNI and Host — test a specific backend
curl -w 'dns=%{time_namelookup} tcp=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total}\n' \
     -o /dev/null -s https://host/   # break the request time down by stage: where is it slow?

# traffic capture
tcpdump -i any -nn port 443 and host 10.0.1.5        # watch packets live
tcpdump -i any -nn -w dump.pcap port 5432             # write to a .pcap file (the standard capture format) for Wireshark — a graphical traffic analyser
```
**`curl -w` with stages** — a strong move in an interview: if `time_namelookup` is long — DNS, `time_connect` — network or TCP, `time_appconnect` — TLS, and a big gap before `time_starttransfer` (TTFB, time to first byte — the time until the first byte of the response) means the application itself is thinking.

### 🧪 Mini task

**Task.** Walk through the "service doesn't respond" diagnostic steps on a local server: who listens on the port (`lsof`), whether the port is open (`nc -z`), how long the connection and the first response byte take (`curl -w`). Then stop the server and check the port again.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
python3 -m http.server 8772 --bind 127.0.0.1 > /dev/null 2>&1 & srv=$!; sleep 1
lsof -nP -iTCP:8772 -sTCP:LISTEN | awk 'NR > 1 {print $1, $9}'
nc -z 127.0.0.1 8772 && echo "port 8772 open"
curl -s -o /dev/null -w 'code=%{http_code} connect=%{time_connect}s ttfb=%{time_starttransfer}s\n' http://127.0.0.1:8772/
kill $srv; sleep 1
nc -z 127.0.0.1 8772 || echo "port 8772 closed"
```

Expected output:

```text
Python 127.0.0.1:8772
port 8772 open
code=200 connect=0.000312s ttfb=0.002144s
port 8772 closed
```

Breakdown: go bottom-up. `lsof` shows which process listens on which address: a common mistake is a service listening on `127.0.0.1` while clients come from outside — then you need `0.0.0.0`. On Linux `ss -ltnp` does the same. `nc -z` checks only the TCP connection. `curl -w` splits the time: a large `connect` points to the network or firewall, a large gap between `connect` and `ttfb` points to a slow application. Your timings will differ; on localhost they are fractions of a millisecond. On Linux the process name will be `python3`.

</details>

## Interview questions

**What happens when you type `https://example.com` in the browser and press Enter?**
Walk through it layer by layer:
1. The browser parses the URL and checks its cache.
2. **DNS**: the browser and OS caches, `/etc/hosts`, then the recursive resolver (root → `.com` → authoritative server); it gets the IP and caches it for the TTL.
3. **Routing**: the IP is not in the local network, so the packet goes to the default gateway. That requires the gateway's MAC — ARP. The home router does SNAT.
4. **TCP**: the three-way handshake with the server on port 443.
5. **TLS**: ClientHello with SNI, the server presents its certificate, the client checks the chain, expiry and name, keys are negotiated.
6. **HTTP**: a `GET /` request with a `Host` header. Along the way there may be a CDN (Content Delivery Network — a network of servers around the world that serves content from the point nearest to the user), a load balancer (TLS termination, backend choice), the application, the database.
7. The response, page rendering, additional requests for CSS, JS and images, often reusing the connection (keep-alive, HTTP/2).

The question tests your overall understanding of the architecture, and then you are asked to expand any component you mentioned. So it helps to be able to continue the request's path **inside the cloud and Kubernetes**:
- **DNS decides which hops are in the path.** If the domain points to CloudFront, the request starts at the CDN; if it points straight to the ALB (common for APIs), CloudFront is not in the path at all;
- **the CDN** (CloudFront) serves cached content (static files) right from an edge location (a CDN server in the city nearest to the user) — such a request never reaches the application. Only cache misses and uncacheable things go to the origin (the source — your real server or bucket the CDN takes content from): modifying methods (POST, PUT), API responses, personal pages;
- the **cloud load balancer** (ALB or NLB) accepts the connection, often terminates TLS and picks a healthy target by health check;
- **from here the path depends on the ingress controller:**
  - **AWS Load Balancer Controller in IP mode** (typical for EKS): the controller **isn't in the traffic path** — it only configures the ALB from Ingress objects (an Ingress is a Kubernetes object with rules "this domain and path → this Service"). The ALB picks a target group by its rules (domain, path), where pod IPs are registered, and sends the request **straight to the pod**. Neither NodePort, nor kube-proxy, nor ClusterIP take part here; the Service is only needed as the source of the pod list;
  - **instance mode**: ALB → NodePort on any node → kube-proxy → pod (possibly on another node) — an extra network hop;
  - **ingress-nginx** and other proxy controllers: NLB → controller pods → **straight to the application pod IPs** from the EndpointSlice (a Kubernetes object with the list of addresses of a service's ready pods), also bypassing ClusterIP;
- the **pod**: the application handles the request, talks to the cache and database, builds the response;
- the **return path** — over the same connections in reverse order.

So where does a Service via kube-proxy actually work? Mostly **inside the cluster**: a pod calls `orders.prod.svc`, CoreDNS returns the ClusterIP (the service's virtual IP, which belongs to no machine and exists only in iptables/IPVS rules on the nodes), and kube-proxy (iptables, IPVS) or eBPF on the calling pod's node rewrites it to the IP of one of the ready pods. Plus the NodePort and instance modes. Distinguishing where the traffic itself flows (data plane) from what only configures its route (control plane: the controller, the Ingress, the Service as a description) is a strong signal in an interview. All the variants step by step, with manifests and verification commands, are in the Kubernetes topic, section "How an external request reaches a pod".

At each step you can name what can go wrong: DNS returns an old address (TTL), the certificate has expired, all targets are unhealthy (503), no Ingress rule for the domain or path (404), no ready pods in the target group or behind the Service, the application doesn't respond within the timeout (504).

**How does TCP differ from UDP? Where is each used?**
TCP is connection-oriented, guarantees delivery, ordering and integrity, and has flow and congestion control; HTTP/1–2, SSH, databases. UDP has no connection and no guarantees, with minimal overhead; DNS, video and voice, NTP, QUIC.

**How many hosts are in a /27 network? What is the mask?**
2^(32−27) = 32 addresses, 30 hosts, mask 255.255.255.224. In AWS 27 are usable (5 addresses are reserved).

**Split 10.0.0.0/16 into 4 equal subnets.**
You need 2 extra bits → /18: `10.0.0.0/18`, `10.0.64.0/18`, `10.0.128.0/18`, `10.0.192.0/18`.

**Which network is the address 172.16.45.200/21 in?**
/21 — the boundary is in the third octet, mask 248, block 8. 45 falls into the 40–47 block. Network `172.16.40.0/21`, broadcast `172.16.47.255`.

**How does a switch differ from a router?**
A switch works at L2: it forwards frames by MAC address within one network. A router works at L3: it forwards packets between different networks by IP and the routing table.

**What is ARP and why is it needed?**
A protocol that finds a neighbour's MAC address in the local network from its IP address (a broadcast request and a reply). It is needed because Ethernet frames are delivered by MAC, while the application only knows the IP.

**What is NAT? SNAT and DNAT?**
Rewriting addresses in packets on a router while remembering the mapping. SNAT changes the source address: private hosts go out to the internet under a public address (NAT Gateway). DNAT changes the destination address: port forwarding to an internal host (`docker -p`, kube-proxy).

**A server doesn't respond. How will you find the problem?**
Bottom-up through the layers:
1. Does the name resolve (`dig`), is it the right IP?
2. Is there a route (`ip route get`), where are packets lost (`mtr`)?
3. Is the port open (`nc -zv`): *refused* — the network is there but the service isn't listening; *timeout* — firewall, Security Group, NACL or routing.
4. On the server: is the process listening (`ss -tlpn`) and on which address — `127.0.0.1` is only reachable locally, you need `0.0.0.0`.
5. TLS (`openssl s_client`), HTTP (`curl -v`), application logs.
6. `tcpdump` on both sides: do packets arrive and do replies go out.

A real-world example — what these steps look like for an actual site: the static cibexon.fyi behind Cloudflare. The layout:
```
Browser → DNS (NS at Cloudflare) → Cloudflare edge: TLS, cache, proxy
        → HTTP → S3 website endpoint (us-east-2) → object in the cibexon.fyi bucket
```
There is no server of your own here, so the "on the server" steps (`ss -tlpn`, logs, server-side tcpdump) are replaced by checking Cloudflare and S3. The commands are for macOS: instead of `ip route get` use `route -n get`, instead of `ss` use `lsof -i` or `netstat`.

Step 0 — a quick assessment: does the site respond and at which stage is time spent. A large `dns` means DNS, `tcp` — the network, `tls` — TLS, a big gap before `ttfb` — Cloudflare or S3. If everything is fast with code 200 and the site is "broken" only for you, the problem is in the browser (step 7).
```bash
curl -sS -o /dev/null -w 'code=%{http_code} dns=%{time_namelookup} tcp=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total}\n' https://cibexon.fyi/
curl -sv https://cibexon.fyi/ -o /dev/null 2>&1 | grep -E '^(\*|<|>)' | head -40
```
Step 1 — DNS. The NS must be at Cloudflare (`jim`, `veronica.ns.cloudflare.com`), the A records — Cloudflare addresses (104.21.x.x, 172.67.x.x): Cloudflare works as a proxy — from outside only it is visible, and it goes to S3 itself. If the public resolvers 1.1.1.1 and 8.8.8.8 answer but the local one doesn't, the problem is in the local cache or at the provider. `+trace` shows where the chain from the root breaks. `whois` — whether the domain has expired. If S3 addresses show up instead of Cloudflare's, proxying is off in Cloudflare (the grey cloud in the DNS record settings), and HTTPS doesn't work: the S3 website endpoint (the address at which S3 serves a bucket as a website) has no HTTPS. The last command flushes the macOS DNS cache.
```bash
dig +short NS cibexon.fyi
dig +short A cibexon.fyi
dig +short A cibexon.fyi @1.1.1.1
dig +short A cibexon.fyi @8.8.8.8
dig +trace cibexon.fyi
whois cibexon.fyi | grep -iE 'expir|status'
sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder
```
Step 2 — the network to Cloudflare: which gateway the traffic goes through, where packets are lost, whether ports 443 and 80 are open. *succeeded* — the port is reachable, *timeout* — the network or a firewall along the way.
```bash
route -n get cibexon.fyi
mtr -rwc 50 cibexon.fyi
nc -zv cibexon.fyi 443
nc -zv cibexon.fyi 80
```
Step 3 — TLS. The edge certificate is issued and renewed by Cloudflare itself (Universal SSL): check the issuer, the expiry date and that the SAN contains `cibexon.fyi` and `*.cibexon.fyi`. Problems are fixed in Cloudflare: SSL/TLS → Edge Certificates.
```bash
echo | openssl s_client -connect cibexon.fyi:443 -servername cibexon.fyi 2>/dev/null \
  | openssl x509 -noout -issuer -enddate -ext subjectAltName
```
Step 4 — the Cloudflare layer. `server: cloudflare` — the response went through Cloudflare; `x-amz-*` headers — the request reached S3. `cf-cache-status`: `HIT` — from cache, `MISS` or `EXPIRED` — fetched from S3, `DYNAMIC` — not cached (HTML). A `?nocache=` parameter with a unique value bypasses the cache. The third command checks whether Cloudflare itself has an outage.
```bash
curl -sI https://cibexon.fyi/ | grep -iE '^(HTTP|server|cf-ray|cf-cache-status|age|x-amz)'
curl -sI "https://cibexon.fyi/site.js?nocache=$(date +%s)"
curl -s https://www.cloudflarestatus.com/api/v2/status.json | python3 -m json.tool | grep description
```
Cloudflare error codes immediately tell you which segment has the problem:

| Code | What it means in this layout |
|---|---|
| 521 | S3 refused the connection |
| 522 | Connection to S3 timed out |
| 523 | Origin unreachable — a wrong address in the Cloudflare DNS record |
| 525, 526 | A TLS error to the origin. Here it means the SSL mode in Cloudflare was switched from Flexible (browser → Cloudflare over HTTPS, Cloudflare → origin over HTTP) to Full (HTTPS on both legs), while the S3 website endpoint works only over HTTP |
| 520 | S3 returned an unexpected response |
| 1xxx | A Cloudflare configuration error, e.g. a DNS record pointing nowhere |

An old version of JS or CSS in the browser is the Cloudflare cache: change `?v=` in `<script src>` (a new URL — the cache treats it as a different file) or do a Purge Cache (a forced cache clear in the Cloudflare dashboard).

Step 5 — the origin directly, bypassing Cloudflare: this shows whether Cloudflare or S3 is to blame. The last command repeats the request exactly the way Cloudflare makes it. The S3 website endpoint picks the bucket by the `Host` header, so the bucket must be named exactly like the domain.
```bash
curl -sI http://cibexon.fyi.s3-website.us-east-2.amazonaws.com/
curl -sI http://cibexon.fyi.s3-website.us-east-2.amazonaws.com/interview.html
curl -sI -H 'Host: cibexon.fyi' http://s3-website.us-east-2.amazonaws.com/
```
S3 responses: `403 AccessDenied` — the public read policy is gone or Block Public Access was turned on (an account or bucket setting that forbids any public access on top of policies); `404 NoSuchKey` — no such file; `404 NoSuchBucket` — `Host` doesn't match the bucket name.

Step 6 — the bucket contents: the files are there, `index.html` has the right `ContentType` (with `binary/octet-stream` the browser downloads the file instead of showing it), website hosting is enabled. The last two commands may fail because of the deploy user's limited permissions.
```bash
aws s3 ls s3://cibexon.fyi/ --region us-east-2
aws s3api head-object --bucket cibexon.fyi --key index.html --region us-east-2 \
  --query '{type:ContentType,size:ContentLength,modified:LastModified}'
aws s3api get-bucket-website --bucket cibexon.fyi --region us-east-2
aws s3api get-bucket-policy --bucket cibexon.fyi --region us-east-2
aws s3api get-public-access-block --bucket cibexon.fyi --region us-east-2
```
Step 7 — the browser. DevTools → Network: which request is red and with what code, whether a file came from the browser cache ("disk cache"). DevTools → Console: JS errors (for example, if `prep/index.json` or the `.md` files fail to load, the interview page stays on "Loading…"). A hard reload with `Cmd+Shift+R` and a check in a private window.

Step 8 — capturing traffic is only possible on your own side: you can see whether SYNs go out and replies come back. The content is encrypted, so beyond that use `curl -v`.
```bash
sudo tcpdump -i en0 -nn 'host 104.21.54.172 or host 172.67.140.177' -c 50
```
The final cheat sheet:

| Symptom | Where to look |
|---|---|
| `Could not resolve host` | Step 1 — DNS, domain expiry |
| Timeout on `nc -zv cibexon.fyi 443` | Step 2 — the network, Cloudflare status |
| Certificate error | Step 3 — Edge Certificates in Cloudflare |
| 52x errors | Step 4 — the Cloudflare–S3 link, SSL mode |
| 403 or 404 with `x-amz-*` headers | Steps 5–6 — bucket policy or missing file |
| Works in curl, not in the browser | Step 7 — browser or Cloudflare cache, JS errors |

**How does "connection refused" differ from "connection timed out"?**
Refused — the packet arrived, but nobody is listening on the port and the host replied with RST: the network is fine, the problem is the service. Timeout — no reply at all: packets are dropped by a firewall, Security Group or NACL, there is no route, or the host is unreachable.

**What is the difference between 502, 503 and 504 on a load balancer?**
502 — the backend returned an invalid response or dropped the connection (crashed, wrong port, the application's keep-alive timeout is shorter than the load balancer's). 503 — no healthy backends. 504 — the backend didn't respond within the load balancer's timeout.

**Lots of connections in TIME_WAIT or CLOSE_WAIT — is that a problem?**
TIME_WAIT is a normal state on the side that closed the connection first. If there are tens of thousands of them, the client opens a new connection for every request, and reuse is needed (keep-alive, pool). CLOSE_WAIT means the application doesn't close sockets after the other side has closed the connection: this is a bug that leads to running out of file descriptors.

**What is SNI and why is it needed?**
The server name the client sends at the start of the TLS handshake. It allows many domains with different certificates on one IP: the server picks the certificate before HTTP starts.

**Why does DNS use UDP? When is TCP used?**
The request and response are small, and setting up a TCP connection would take longer than the query itself. TCP is used when the response doesn't fit into a UDP packet (the server returns a truncation flag and the client repeats the query over TCP), for zone transfers between servers (AXFR — copying the whole zone from the primary DNS server to a secondary) and in DNS over TLS (encrypted DNS, port 853).

**Why can't you put a CNAME at the root of a domain?**
The zone root must have SOA and NS records, and by the standard a CNAME cannot coexist with other records. The solution is an Alias in Route 53, ALIAS/ANAME or CNAME flattening at other providers.

**You changed the IP in DNS, but some clients still go to the old address. Why?**
The answer is cached by resolvers for the TTL (plus caches inside applications: for example, the JVM may cache DNS for a long time). Before a migration the TTL has to be lowered in advance.

**How does a Security Group differ from a NACL?**
An SG is stateful, applies to a network interface (instance), has allow rules only, and can reference other SGs. A NACL is stateless (return traffic to ephemeral ports has to be allowed too), applies to a subnet, has both allow and deny rules, evaluated by rule number.

**How does an L4 load balancer differ from an L7 one?**
L4 sees only IPs and ports: fast, protocol-agnostic, can preserve the client IP. L7 understands HTTP: routing by domain and path, TLS termination, headers, WAF, but with more overhead.

**What is MTU and what do problems with it look like?**
The maximum packet size on a link (1500 for Ethernet). If there is a link with a smaller MTU on the path (VPN, tunnel) and ICMP is blocked, large packets are silently lost: the connection is established and small requests work, but large responses hang.

**How do you see how much time goes to DNS, TCP, TLS and the server's response?**
`curl -w` with the variables `time_namelookup`, `time_connect`, `time_appconnect`, `time_starttransfer` and `time_total`.

**Why are small TCP requests sometimes delayed by about 40 ms?**
Nagle's algorithm on the sender waits for the previous small packet to be acknowledged, while delayed ACK on the receiver waits up to ~40 ms to acknowledge together with data. Together they wait for each other. The fix is `TCP_NODELAY` in the application.

**How does a single lost packet affect P99, and how do you notice it?**
If the loss isn't recovered by fast retransmit, the retransmission timeout fires — at least 200 ms in Linux. For a budget of a few milliseconds this is a disaster for the tail. Look at `nstat` (TcpRetransSegs, TCPTimeouts), `ss -ti`, `tcpretrans`, and on EC2 also the ENA allowance counters.

**How do you set up mTLS between hundreds of hosts?**
Your own CA with an offline root and an intermediate issuing CA; machines get short-lived certificates automatically by proving their identity (instance IAM role via Vault or AWS Private CA, SPIFFE/SPIRE); the key is generated on the machine; rotation without restarts; expiry monitoring. Instead of revocation — a short lifetime.

**Why do DNS queries in Kubernetes sometimes answer after exactly 5 seconds?**
5 seconds is the resolver's retry timeout. Usually it's a race in conntrack: glibc sends the A and AAAA queries in parallel from one UDP socket, and one packet is lost in NAT. The second frequent source is an overloaded CoreDNS because of `ndots:5`. Solutions: NodeLocal DNSCache, `single-request-reopen` in `dnsConfig`, a lower `ndots` or a trailing dot in the name, scaling CoreDNS and monitoring its latency.

**How do you set up name resolution between AWS and GCP (or a data centre)?**
Via query forwarding. In AWS — Route 53 Resolver: an outbound endpoint and forwarding rules for the GCP or data-centre zones, and an inbound endpoint — the addresses to which the other side forwards queries for AWS zones. In GCP — a forwarding zone in Cloud DNS pointing at the AWS inbound endpoint and an inbound server policy for the reverse direction. DNS goes over the VPN or Interconnect, so routes and allowed ports 53 UDP and TCP are needed, and the zones mustn't overlap by name.
