package job

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/zrepl/zrepl/config"
)

func JobsFromConfig(c *config.Config) ([]Job, error) {
	js := make([]Job, len(c.Jobs))
	for i := range c.Jobs {
		j, err := buildJob(c.Global, c.Jobs[i])
		if err != nil {
			return nil, err
		}
		if j == nil || j.Name() == "" {
			panic(fmt.Sprintf("implementation error: job builder returned nil job type %T", c.Jobs[i].Ret))
		}
		js[i] = j
	}
	return js, nil
}

func buildJob(c *config.Global, in config.JobEnum) (j Job, err error) {
	cannotBuildJob := func(e error, name string) (Job, error) {
		return nil, errors.Wrapf(e, "cannot build job %q", name)
	}
	// FIXME prettify this
	switch v := in.Ret.(type) {
	case *config.SinkJob:
		m, err := modeSinkFromConfig(c, v)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
		j, err = passiveSideFromConfig(c, &v.PassiveJob, m)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
	case *config.SourceJob:
		m, err := modeSourceFromConfig(c, v)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
		j, err = passiveSideFromConfig(c, &v.PassiveJob, m)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
	case *config.PushJob:
		m, err := modePushFromConfig(c, v)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
		j, err = activeSide(c, &v.ActiveJob, m)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
	case *config.PullJob:
		m, err := modePullFromConfig(c, v)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
		j, err = activeSide(c, &v.ActiveJob, m)
		if err != nil {
			return cannotBuildJob(err, v.Name)
		}
	default:
		panic(fmt.Sprintf("implementation error: unknown job type %T", v))
	}
	return j, nil

}
