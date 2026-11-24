package provider

import (
	"context"
	"errors"
)

// recordOrDiscard records the binding of the pane a launch just created. When
// that record fails it retires the pane under discardLaunchPolicy, so the pane
// does not run on after start's deferred rollback forgets its binding. It
// returns the record error joined with the retirement error. The provisional
// record and the seeded key stay for start's rollback.
func (p *Provider) recordOrDiscard(ctx context.Context, n SessionName, bd Binding) error {
	if recErr := p.binder.Record(bd); recErr != nil {
		_, err := p.retire(ctx, n, bd, discardLaunchPolicy())
		return errors.Join(recErr, err)
	}
	return nil
}
