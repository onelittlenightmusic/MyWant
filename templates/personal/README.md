# MyWant Personal Preset Template Collection

A personal template collection leveraging the Agent/Capability system. Each template utilizes specialized agents that collaborate to execute tasks effectively.

## 📚 Learning Plan Templates
- `learning-language.yaml` - Language learning plan
- `learning-skill.yaml` - Skill acquisition plan
- `learning-certification.yaml` - Certification preparation plan

## 🔄 Habit Improvement Templates
- `habit-morning-routine.yaml` - Morning routine building
- `habit-exercise.yaml` - Exercise habit formation
- `habit-reading.yaml` - Reading habit development

## ✈️ Travel Planning Templates
- `travel-domestic.yaml` - Domestic travel planning
- `travel-international.yaml` - International travel planning
- `travel-business.yaml` - Business trip planning

## 💪 Health Management Templates
- `health-fitness.yaml` - Fitness planning
- `health-nutrition.yaml` - Nutrition management
- `health-mental.yaml` - Mental health care


## 🌟 Beginner Starter Templates
- `starter-goal-setting.yaml` - Goal setting fundamentals
- `starter-time-management.yaml` - Time management basics
- `starter-personal-project.yaml` - Personal project management

## Agent/Capability System Integration

Each template utilizes collaborative agents with specialized capabilities:

### Core Agents
- **PlannerAgent**: Strategic planning and goal setting
- **TrackerAgent**: Progress monitoring and status tracking
- **CoachAgent**: Guidance and motivation management
- **AnalyzerAgent**: Data analysis and improvement recommendations

### Specialized Agents
- **LearningAgent**: Learning plan optimization and resource recommendation
- **HealthAgent**: Health status monitoring and wellness suggestions
- **TravelAgent**: Travel arrangement and information gathering

## Usage

```bash
# Example: Language learning template
make run-template template=templates/personal/learning-language.yaml

# Custom parameters example
make run-template template=templates/personal/learning-language.yaml \
  target_language=English study_hours=2 goal_months=6
```

Each template can be customized according to individual goals and circumstances.