package lowhttp

// This file previously contained H2 pool helpers (markH2Active, markH2Idle,
// h2DialCall) that were embedded in LowHttpConnPool. They have been moved to
// the standalone H2ConnPool in h2_conn_pool.go.
//
// h2DialCall is defined in h2_conn_pool.go.
