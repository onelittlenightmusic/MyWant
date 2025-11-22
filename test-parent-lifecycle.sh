#!/bin/bash

echo "🧪 Testing Parent-Child Lifecycle with Parameter Updates"
echo "========================================================="
echo ""

cd engine

# Compile the test
echo "📦 Building test program..."
go build -o ../bin/test-parent-restart ../test-parent-restart.go

if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi

echo "✅ Build successful"
echo ""

# Run the test
echo "🚀 Running test..."
cd ..
./bin/test-parent-restart

echo ""
echo "✅ Test execution completed"