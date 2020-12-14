# migbee

Migbee is a CLI library for Go that support db migration
to container and node endpoint.This application is a tool 
to migrate the file sql data to multi databases, such as
mysql/mariadb, redis, postgres ...

[![GoDoc Widget]][GoDoc] [![Travis Widget]][Travis]

# Install

    $ git clone https://github.com/codeandcode0x/migrationbee.git
    $ cp migrationbee/bin/[OS]/migbee  /usr/bin
    $ migbee -h

# Usage

```
Usage: migbee DRIVER COMMAND [OPTIONS]

Drivers:
    postgres
    mysql
    sqlite3
    mssql
    redshift

Examples:

    migbee mysql --url "mysql://root:coding123@mysql:3306/dbname" --path source_folders --service mariadb --namespace ns

Options:
  -h  print help

Commands:
    up                   Migrate the DB to the most recent version available
    down                 Roll back the version by 1
    redo                 Re-run the latest migration
    reset                Roll back all migrations
    status               Dump the migration status for the current DB
    version              Print the current version of the database
```




## License

Licensed under [MIT License](./LICENSE)

