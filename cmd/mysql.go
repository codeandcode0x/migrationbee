package cmd

import (
    "github.com/spf13/cobra"
    migration "migrationbee/src/migration"
    "log"
)


var dbUrl, filePath, srcName, nameSpace, apps string

//init
func init() {
    //migration data
    migrationCmd.Flags().StringVarP(&dbUrl, "url", "u", "", "Database url")
    migrationCmd.Flags().StringVarP(&filePath, "path", "p", "", "Location of sql file")
    migrationCmd.Flags().StringVarP(&srcName, "service", "s", "", "K8s service name")
    migrationCmd.Flags().StringVarP(&nameSpace, "namespace", "n", "", "K8s namespace")
    migrationCmd.Flags().StringVarP(&apps, "apps", "a", "all", "applications")
    rootCmd.AddCommand(migrationCmd)
}

//register command
var migrationCmd = &cobra.Command{
    Use:   "mysql",
    Short: "Migration data to mysql",
    Long: `Migration data to mysql`,
    Run: func(cmd *cobra.Command, args []string) {
        //TO-DO
        log.Println("welcome to use migbee to migrate data")
        migration.DeployResourceByLayNodes(apps, "true", "MYSQL", dbUrl, filePath, srcName, nameSpace)
    },
}







