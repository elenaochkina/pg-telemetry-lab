package benchmark

type PgBenchOptions struct {
    Name     string 
    Port     int
    User     string
    Database string

    Duration int
    Clients  int
    Scale    int
    Progress int
}

type Runner interface {
    Init(opts PgBenchOptions) error
    Run(opts PgBenchOptions) error
}

