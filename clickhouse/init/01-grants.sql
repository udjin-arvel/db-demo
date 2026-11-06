-- Allow the demo app user to read size/explain-style stats from system tables.
GRANT SELECT ON system.query_log TO demo;
GRANT SELECT ON system.parts TO demo;
