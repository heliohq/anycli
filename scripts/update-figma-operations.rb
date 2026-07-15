#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "yaml"

unless ARGV.length == 2
  warn "usage: ruby scripts/update-figma-operations.rb OPENAPI_YAML SOURCE_REVISION"
  exit 2
end

spec_path, source_revision = ARGV
spec = YAML.safe_load(File.read(spec_path), aliases: true)
operations = []

spec.fetch("paths").each do |path, path_item|
  path_item.each do |method, operation|
    next unless %w[get post put patch delete].include?(method)

    security = operation.fetch("security", [])
    parameters = path_item.fetch("parameters", []) + operation.fetch("parameters", [])
    operations << {
      "id" => operation.fetch("operationId"),
      "method" => method.upcase,
      "path" => path,
      "summary" => operation.fetch("summary"),
      "pat" => security.any? { |scheme| scheme.key?("PersonalAccessToken") },
      "scopes" => security.flat_map(&:values).flatten.compact.reject { |scope| scope == "files:read" }.uniq.sort,
      "path_params" => parameters.map { |parameter| parameter["name"] if parameter["in"] == "path" }.compact,
      "query_params" => parameters.map { |parameter| parameter["name"] if parameter["in"] == "query" }.compact,
      "required_query_params" => parameters.map do |parameter|
        parameter["name"] if parameter["in"] == "query" && parameter["required"] == true
      end.compact,
      "body_required" => operation.dig("requestBody", "required") == true
    }
  end
end

catalog = {
  "source" => source_revision,
  "operations" => operations
}
output_path = File.expand_path("../internal/tools/figma/operations.json", __dir__)
File.write(output_path, JSON.pretty_generate(catalog) + "\n")
warn "wrote #{operations.length} operations to #{output_path}"
