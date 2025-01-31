# About

**Postgres Chaos Analyst** is a tool meant to analyse the behavior of a terraform-managed patroni cluster from a client's perspective as the following scenarios are triggered in a development environment:
  - Leadership change trigged via the Patroni api
  - Destroying and recreating the leader node
  - Destroying and recreating the sync standby node

The tool will throw a flurry of basic update transactions at the postgres cluster while those disruptions are happening in the background and will take note and compile a report on the following events:
- Observed downtime
- Lost transactions
- Ghost transactions (ie, transaction that returned an error, but were commited anyways)

Also, the tool will monitor the evolving status of the patroni cluster using the patroni api and will abord in failure if the patroni cluster does not fully recover within a specified amount of time after each disruption.

# Requirements

To use the tool, you need a terraform orchestrated patroni cluster where you can create/destroy any of the postgres servers using a yaml file in the terraform directory that follows the following format:
```
cluster:
- name: <Name of first postgres server>
  up: true|false
- name: <Name of second postgres server>
  up: true|false
...

Before running the postgres chaos analyst, you need to provision your cluster will all the servers being up.

Additionally, you need to create a database in the postgres cluster with the right credentials for **Postgres Chaos Analyst** to use.

# Usage

...