# About

**Postgres Chaos Analyst** is a tool meant to analyse the behavior of a terraform-managed patroni cluster from a client's perspective as the following scenarios are triggered in a development environment:
  - Leadership change trigged via the Patroni api
  - Destroying and recreating the leader node
  - Destroying and recreating the sync standby node

The tool will throw a flurry of basic update transactions at the postgres cluster while those disruptions are happening in the background and will take note and compile a report on the following events:
- Observed downtime
- Lost transactions
- Ghost transactions (ie, transaction that returned an error, but were commited anyways)

Also, the tool will monitor the evolving status of the patroni cluster using the patroni api and will abort in failure if the patroni cluster does not fully recover within a specified amount of time after each disruption.

# Requirements

To use the tool, you need a terraform orchestrated patroni cluster where you can create/destroy any of the postgres servers using a yaml file in the terraform directory that follows the following format:
```
cluster:
- name: <Name of first postgres server>
  up: true|false
- name: <Name of second postgres server>
  up: true|false
...
```

Before running the postgres chaos analyst, you need to provision your cluster will all the servers being up.

Additionally, you need to create a database in the postgres cluster with the right credentials for **Postgres Chaos Analyst** to use.

# Configuration

The behavior of the tool can configured by a configuration file whose path can be set with the **PG_CHAOS_ANALYST_CONFIG_FILE** environment variable and which defaults to a file named **config.yml** located in the process' working directory.

The file has the following keys:

- **postgres_client**:
  - **endpoint**: Postgres endpoint which should be formated as `<host>:<port>`
  - **auth**:
    - **ca_cert**: Path to a CA certification that should be use to authentify postgres' server certificate.
    - **password_auth**: Path to a yaml containing a **username** and **password** key used to authentify with posgres.
  - **database**: Postgres database to connect to
  - **connection_timeout**: Timeout to connect to the postgres server
  - **query_timeout**: Timeout for queries on the postgres server
- **terraform**: 
  - **directory**: Directory where the terraform orchestration files for the postgres cluster is located.
  - **cluster_file**: Name of the yaml cluster status file that the tool will us to bring destroy and re-create members of the patroni cluster. 
- **patroni_client**:
  - **endpoint**: Patroni endpoint which should be formated as `<host>:<port>`
  - **auth**:
    - **ca_cert**: Path to a CA certification that should be use to authentify patroni's server certificate.
    - **client_cert**: Path to client certificate the tool will use to authentify itself to patroni
    - **client_key**: Path to client key the tool will use to authentify itself to patroni
  - **request_timeout**: Timeout for requests on the patroni server
- **tests**:
  - **switchovers**: Number of patroni leader switchover requests to make the patroni api as part of the tests.
  - **leader_crashes**: Number of times to destroy and recreate the patroni leader as part of the tests.
  - **sync_standby_crashes**: Number of times to destroy and recreate the synchronous standby server as part of the tests.
  - **validation_interval**: Duration to wait after the recovery of a disruptive action before performing the next one.
  - **change_recover_timeout**: Timeout to give the patroni cluster to fully recover from a leadership change request.
  - **crash_recover_timeout**: Timeout to give the patroni cluster to fully recover after a member has been destroyed and rebuild. Setup delays to create a patroni member should be factored in when setting this timeout.
  - **crash_rebuild_pause**: Wait period before triggering the re-creation of a patroni member after its destruction. Can be useful to better observe the client experience on a partially available cluster if you have a setup where patroni members can be re-created very quickly.