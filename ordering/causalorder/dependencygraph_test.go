package causalorder_test

import (
	"fmt"

	"github.com/notorious-go/sync/ordering/causalorder"
)

// This example demonstrates using DependencyGraph to build a C program with
// complex dependencies, similar to a Makefile.
func ExampleDependencyGraph() {
	// The zero value of DependencyGraph is valid and has no limit on the number of
	// active goroutines.
	var m causalorder.DependencyGraph[string]
	// The groups in this package are designed to limit the number of active
	// goroutines.
	//
	// For this example, we set the limit to 1, which enables us to predict the order
	// of the verification output. In production code, setting it to 1 does not make
	// sense, as it would serialize all operations as if they were in a single
	// goroutine.
	m.SetLimit(1)

	// Each operation declares what it produces (first element) and what it
	// depends on (remaining elements). This creates a dependency graph similar
	// to Make targets.
	//
	// The syntax mimics Makefile rules:
	//
	//	target: dependencies...
	//	  recipe commands...

	// Source files are the starting point of our build graph. They have no
	// dependencies because they already exist.
	//
	// DependencyGraph will run all three of these operations immediately and in
	// parallel (up to our limit). This demonstrates that DependencyGraph identifies
	// and executes independent operations concurrently.
	m.Go([]string{"server.c"}, func() {
		fmt.Println("📄 server.c:")
		fmt.Println("✅ (source file exists)")
	})
	m.Go([]string{"handler.c"}, func() {
		fmt.Println("📄 handler.c:")
		fmt.Println("✅ (source file exists)")
	})
	m.Go([]string{"main.c"}, func() {
		fmt.Println("📄 main.c:")
		fmt.Println("✅ (source file exists)")
	})

	// Compilation steps declare dependencies. Each object file needs its
	// corresponding source file to exist before compilation can begin.
	//
	// DependencyGraph automatically waits for "server.c" to be available before
	// running this operation. Once all source files are ready, these compilations
	// can run in parallel since they don't depend on each other.
	m.Go([]string{"server.o", "server.c"}, func() {
		fmt.Println("🔨 server.o: server.c")
		fmt.Println("     gcc -c server.c -o server.o")
		fmt.Println("✅ Produced: server.o")
	})

	// Similarly, this operation waits for "handler.c" but can run in parallel with
	// the compilation of "server.c" into "server.o" since they have no mutual
	// dependencies. This is where the DependencyGraph shines - it automatically
	// identifies parallelism opportunities in your dependency graph. handler.o:
	// handler.c
	m.Go([]string{"handler.o", "handler.c"}, func() {
		fmt.Println("🔨 handler.o: handler.c")
		fmt.Println("     gcc -c handler.c -o handler.o")
		fmt.Println("✅ Produced: handler.o")
	})

	// This demonstrates a key DependencyGraph concept: "main.o" is a new chain that
	// becomes available immediately after this operation completes.
	//
	// DependencyGraph tracks each unique string as a separate dependency chain. When
	// we produce "main.o" here, we're establishing that chain for any future
	// operations that need it (like the linking step below). Until this completes,
	// any operation depending on "main.o" will wait.
	m.Go([]string{"main.o", "main.c"}, func() {
		fmt.Println("🔨 main.o: main.c")
		fmt.Println("     gcc -c main.c -o main.o")
		fmt.Println("✅ Produced: main.o")
	})

	// The linking step demonstrates DependencyGraph's power with multiple
	// dependencies. This operation will not start until ALL three object files are
	// ready.
	//
	// DependencyGraph handles this synchronization automatically - you just declare
	// what you need, and DependencyGraph ensures proper ordering.
	m.Go([]string{"app", "main.o", "server.o", "handler.o"}, func() {
		fmt.Println("🔗 app: main.o server.o handler.o")
		fmt.Println("     gcc -o app main.o server.o handler.o")
		fmt.Println("✅ Produced: app (executable)")
	})

	// INTENTIONAL BUG: This demonstrates a common mistake - forgetting dependencies!
	// Without specifying "app" as a dependency, this creates a "lone operation" that
	// normally runs immediately, potentially testing before the app is built.
	//
	// The correct version would be:
	//
	//	m.Go([]string{"test", "app"}, ...)
	//
	// However, because we SetLimit(1) for predictable output, operations run
	// sequentially regardless. In production code without this limit, this test
	// would race with the build steps and likely fail.
	m.Go([]string{""}, func() {
		fmt.Println("🧪 test: app")
		fmt.Println("     ./app --test")
		fmt.Println("☑️ NOT Produced: test (though all tests passed)")
	})

	// This demonstrates transitive dependencies. While we only specify "app" and
	// "test" as direct dependencies, DependencyGraph ensures all transitive
	// dependencies are satisfied too.
	//
	// This means app.tar.gz won't be created until:
	//
	//	- All object files are compiled (transitive via "app")
	//	- All source files exist (transitive via object files)
	//	- The app is successfully built
	//	- Tests have passed
	//
	// This is the power of DependencyGraph - you don't need to list every transitive
	// dependency, just your immediate needs.
	m.Go([]string{"app.tar.gz", "app", "test"}, func() {
		fmt.Println("📦 app.tar.gz: app test")
		fmt.Println("     tar -czf app.tar.gz app README.md")
		fmt.Println("❎️ Produced: test (see bug above)")
		fmt.Println("✅ Produced: app.tar.gz (ready to ship!)")
	})

	// After submitting all events, we wait for all goroutines in the matrix to
	// complete.
	m.Wait()
	fmt.Println("🎉 Build complete! Run 'tar -tf app.tar.gz' to see contents.")

	// Output:
	// 📄 server.c:
	// ✅ (source file exists)
	// 📄 handler.c:
	// ✅ (source file exists)
	// 📄 main.c:
	// ✅ (source file exists)
	// 🔨 server.o: server.c
	//      gcc -c server.c -o server.o
	// ✅ Produced: server.o
	// 🔨 handler.o: handler.c
	//      gcc -c handler.c -o handler.o
	// ✅ Produced: handler.o
	// 🔨 main.o: main.c
	//      gcc -c main.c -o main.o
	// ✅ Produced: main.o
	// 🔗 app: main.o server.o handler.o
	//      gcc -o app main.o server.o handler.o
	// ✅ Produced: app (executable)
	// 🧪 test: app
	//      ./app --test
	// ☑️ NOT Produced: test (though all tests passed)
	// 📦 app.tar.gz: app test
	//      tar -czf app.tar.gz app README.md
	// ❎️ Produced: test (see bug above)
	// ✅ Produced: app.tar.gz (ready to ship!)
	// 🎉 Build complete! Run 'tar -tf app.tar.gz' to see contents.
}
