# Get version from git tag (e.g., "v1.0.0" or "v1.0.0-3-g1a2b3c4")
def git_version
  version = `git describe --tags --always --match 'v*' 2>/dev/null`.strip
  version.empty? ? "dev" : version.sub(/^v/, "")
end

@goxslt_version = git_version

desc "Show rake description"
task :default do
    puts
    puts "Run 'rake -T' for a list of tasks."
    puts
    puts "Use 'rake build' to build the 'goxslt' binary."
    puts
end

desc "Build the 'goxslt' binary"
task :build do
    sh "go build -ldflags '-s -w -X main.Version=#{@goxslt_version}' -o bin/goxslt github.com/speedata/goxslt/cmd/goxslt"
end

desc "Install 'goxslt' into $GOBIN"
task :install do
    sh "go install -ldflags '-s -w -X main.Version=#{@goxslt_version}' github.com/speedata/goxslt/cmd/goxslt"
end

desc "Show version information"
task :showversion do
    puts "goxslt version #{@goxslt_version}"
end
