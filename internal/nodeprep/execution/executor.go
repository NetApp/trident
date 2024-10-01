package execution

import (
	"context"

	"github.com/netapp/trident/internal/nodeprep/instruction"
	. "github.com/netapp/trident/logging"
)

func Execute(ctx context.Context, instructions []instruction.Instructions) (err error) {
	for _, i := range instructions {
		Log().WithField("instructions", i.GetName()).Info("Preparing node")
		if err = i.PreCheck(ctx); err != nil {
			return
		}
		if err = i.Apply(ctx); err != nil {
			return
		}
		if err = i.PostCheck(ctx); err != nil {
			return
		}
	}
	return
}
