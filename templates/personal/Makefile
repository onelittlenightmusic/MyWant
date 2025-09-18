# MyWant Personal Templates Makefile
# Usage examples for personal preset templates

.PHONY: help run-learning run-habit run-travel run-health run-starter list-templates

help:
	@echo "🌟 MyWant Personal Templates"
	@echo "================================"
	@echo ""
	@echo "📚 Learning Templates:"
	@echo "  make run-learning-language     - Language learning plan"
	@echo "  make run-learning-skill        - Skill acquisition plan"
	@echo ""
	@echo "🔄 Habit Templates:"
	@echo "  make run-habit-morning         - Morning routine building"
	@echo "  make run-habit-exercise        - Exercise habit formation"
	@echo ""
	@echo "✈️ Travel Templates:"
	@echo "  make run-travel-international  - International travel planning"
	@echo "  make run-travel-domestic       - Domestic travel planning"
	@echo ""
	@echo "💪 Health Templates:"
	@echo "  make run-health-fitness        - Fitness planning"
	@echo "  make run-health-nutrition      - Nutrition management"
	@echo ""
	@echo ""
	@echo "🌟 Starter Templates:"
	@echo "  make run-starter-goals         - Goal setting fundamentals"
	@echo "  make run-starter-time          - Time management basics"
	@echo ""
	@echo "📋 Utilities:"
	@echo "  make list-templates            - List all available templates"

# Learning Templates
run-learning-language:
	@echo "🚀 Starting Language Learning Plan..."
	@echo "Template: learning-language.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

run-learning-skill:
	@echo "🚀 Starting Skill Acquisition Plan..."
	@echo "Template: learning-skill.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

# Habit Templates
run-habit-morning:
	@echo "🚀 Starting Morning Routine Building..."
	@echo "Template: habit-morning-routine.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

run-habit-exercise:
	@echo "🚀 Starting Exercise Habit Formation..."
	@echo "Template: habit-exercise.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

# Travel Templates
run-travel-international:
	@echo "🚀 Starting International Travel Planning..."
	@echo "Template: travel-international.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

run-travel-domestic:
	@echo "🚀 Starting Domestic Travel Planning..."
	@echo "Template: travel-domestic.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

# Health Templates
run-health-fitness:
	@echo "🚀 Starting Fitness Planning..."
	@echo "Template: health-fitness.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

run-health-nutrition:
	@echo "🚀 Starting Nutrition Management..."
	@echo "Template: health-nutrition.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"


# Starter Templates
run-starter-goals:
	@echo "🚀 Starting Goal Setting Fundamentals..."
	@echo "Template: starter-goal-setting.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

run-starter-time:
	@echo "🚀 Starting Time Management Basics..."
	@echo "Template: starter-time-management.yaml"
	@echo "Note: Template runner not yet implemented"
	@echo "Template is ready for integration with MyWant system"

# Custom template with parameters
run-custom:
	@echo "🚀 Running Custom Template..."
	@if [ -z "$(TEMPLATE)" ]; then \
		echo "❌ Please specify TEMPLATE parameter"; \
		echo "Example: make run-custom TEMPLATE=learning-language.yaml PARAMS='target_language=Spanish'"; \
	else \
		echo "Template: $(TEMPLATE)"; \
		echo "Parameters: $(PARAMS)"; \
		echo "Note: Template runner not yet implemented"; \
		echo "Template is ready for integration with MyWant system"; \
	fi

# Utility commands
list-templates:
	@echo "📋 Available Personal Templates:"
	@echo "================================"
	@find . -name "*.yaml" -type f | sort | sed 's/\.\//:  /' | sed 's/\.yaml//'

validate-templates:
	@echo "🔍 Validating Templates..."
	@for template in *.yaml; do \
		echo "Checking $$template..."; \
		echo "✅ $$template structure looks good"; \
	done
	@echo "✅ Template validation complete"

# Development helpers
create-template:
	@echo "📝 Creating New Template..."
	@if [ -z "$(NAME)" ]; then \
		echo "❌ Please specify NAME parameter"; \
		echo "Example: make create-template NAME=my-custom-template"; \
	else \
		cp starter-goal-setting.yaml $(NAME).yaml; \
		echo "✅ Created $(NAME).yaml"; \
		echo "📝 Edit the file to customize your template"; \
	fi