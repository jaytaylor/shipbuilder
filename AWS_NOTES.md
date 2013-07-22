AWS Instance Type Recommendations

Build Box: m1.large with 100GB EBS volume for persistent BTRFS mount; Reasonable CPU power for compiling apps, and high network throughput (500mbps) for pushing images out quickly

Load Balancer: c1.medium; CPU-per-dollar

Nodes: m2.4xlargem2.4xlarge; Use anything which can run the requisite number of instances your app(s)

