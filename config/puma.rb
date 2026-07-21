require_relative "../lib/puma_chunked_body_limit"

http_content_length_limit 64 * 1024 * 1024

on_booted { Gripi.start_rpc_client_maintenance }
on_stopped { Gripi.stop_rpc_client_maintenance }
