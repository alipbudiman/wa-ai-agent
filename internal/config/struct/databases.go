package cfg_struct

type MongoDBConfig struct {
	URI     string `mapstructure:"uri"`
	DBAlias string `mapstructure:"db_alias"`
	DBName  string `mapstructure:"db_name"`
}

type DatabaseConfig struct {
	MongoDB MongoDBConfig `mapstructure:"mongodb"`
}
