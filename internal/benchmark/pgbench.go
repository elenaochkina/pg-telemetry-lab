package benchmark

// PgBenchOptions defines all configuration parameters required to execute
// a pgbench benchmark run (both initialization and workload phases).
// These options are passed to a Runner implementation which executes
// pgbench either locally or inside a Docker container.
type PgBenchOptions struct {

    // Name is the hostname or container name of the PostgreSQL instance
    // pgbench should connect to. Example: "pg-primary".
    HostName string

    // Port is the TCP port of the PostgreSQL instance pgbench connects to.
    // Example: 5432.
    Port int

    // User is the PostgreSQL user used for authentication when running pgbench.
    // Typically "postgres" or another superuser.
    User string

    // Database is the target database name for pgbench connection and workload.
    // Example: "pgbench".
    Database string

    // Duration is the total benchmark runtime in seconds (for workload runs).
    // This maps to the pgbench flag `-T`.
    Duration int

    // Clients is the number of concurrent pgbench clients (sessions).
    // This maps to the pgbench flag `-c`.
    Clients int

    // Scale defines the dataset size for initialization.
    // This maps to the pgbench flag `-s`.
    // Higher scale generates proportionally larger pgbench tables.
    Scale int

    // Progress is the number of seconds between progress report outputs.
    // This maps to the pgbench flag `-P`. A value of 0 disables progress output.
    Progress int
}

// Runner is the interface used to execute pgbench workloads.
// Implementations define how pgbench is invoked—for example, using Docker,
// running pgbench directly on the host, or remotely via SSH.
type Runner interface {

    // Init initializes the pgbench schema and loads initial test data.
    // This corresponds to the pgbench initialization phase (`pgbench -i`),
    // which creates standard tables (pgbench_accounts, pgbench_branches, etc.)
    // and populates them based on the Scale factor.
    Init(opts PgBenchOptions) error

    // Run executes the actual pgbench benchmark workload using the parameters
    // defined in opts (clients, duration, progress, etc.). This corresponds to
    // a pgbench invocation without `-i`, such as:
    //     pgbench -T <duration> -c <clients> -P <progress> <database>
    Run(opts PgBenchOptions) error
}


