# AWS RDS Reserved Instances Pricing Comparison Tool

This tool is designed to fetch and compare pricing data for AWS RDS instance Reserved Instances of the instances you're running in your account.

It shows both on-demand and the available reserved instance pricing in a table format, helping users make informed decisions about their
reserved instance purchases.

The table is formatted as Markdown to make it easy to share in text form.

## Screenshot

![image](https://github.com/LeanerCloud/aws-reserved-instances-cost-comparison/assets/95209/f5897e35-6227-4be7-aae6-2139f06e97e2)


## Getting Started

These instructions will help you set up and run the tool on your local machine for development and testing purposes.

### Prerequisites

- Go (Version 1.x)
- AWS Account and AWS SDK configured with access to RDS pricing information.

### Installing

```sh
go install github.com/yourusername/aws-rds-pricing-tool@latest
```

## Usage

Run the command in a shell that has the required AWS environment variables set for authentication with an account.

```sh
./aws-rds-pricing-tool -region <aws-region> [-logLevel (info)/error/debug]
```

## Related Projects

Check out our other FinOps open-source [projects](https://github.com/LeanerCloud)

- [awesome-finops](https://github.com/LeanerCloud/awesome-finops) - a more up-to-date and complete fork of [jmfontaine/awesome-finops](https://github.com/jmfontaine/awesome-finops).
- [Savings Estimator](https://github.com/LeanerCloud/savings-estimator) - estimate Spot savings for ASGs.
- [AutoSpotting](https://github.com/LeanerCloud/AutoSpotting) - convert On-Demand ASGs to Spot without config changes, automated divesification, and failover to On-Demand.
- [EBS Optimizer](https://github.com/LeanerCloud/EBSOptimizer) - automatically convert EBS volumes to GP3.
- [ec2-instances-info](https://github.com/LeanerCloud/ec2-instances-info) - Golang library for specs and pricing information about AWS EC2 instances based on the data from [ec2instances.info](https://ec2instances.info).

For more advanced features of some of these tools, as well as comprehensive cost optimization services focused on AWS, visit our commercial offerings at [LeanerCloud.com](https://www.LeanerCloud.com).

We're also working on an automated RDS rightsizing tool that converts DBs to Graviton instance types and GP3 storage. If you're interested to learn more about it, reach out to us on [Slack](https://join.slack.com/t/leanercloud/shared_invite/zt-xodcoi9j-1IcxNozXx1OW0gh_N08sjg).

## Contributing

We welcome contributions! Please submit PRs or create issues for any enhancements, bug fixes, or features you'd like to add.

## License

This project is licensed under the GNU AFFERO GENERAL PUBLIC LICENSE, see the LICENSE file for details.

Copyright (c) 2023 Cristian Magherusan-Stanciu, [LeanerCloud.com](https://www.LeanerCloud.com).

